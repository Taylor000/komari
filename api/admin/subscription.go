package admin

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/subscriptions"
	"gorm.io/gorm"
)

type subscriptionRequest struct {
	Name         string  `json:"name"`
	Plan         string  `json:"plan"`
	Price        float64 `json:"price"`
	Currency     string  `json:"currency"`
	BillingCycle int     `json:"billing_cycle"`
	ExpiresAt    string  `json:"expires_at"`
	StartedAt    string  `json:"started_at"`
	AutoRenewal  bool    `json:"auto_renewal"`
	Visible      *bool   `json:"visible"`
	ShowPrice    *bool   `json:"show_price"`
	URL          string  `json:"url"`
	Remark       string  `json:"remark"`
}

func normalizeSubscription(req subscriptionRequest) (models.Subscription, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Plan = strings.TrimSpace(req.Plan)
	req.Currency = strings.TrimSpace(req.Currency)
	req.URL = strings.TrimSpace(req.URL)
	req.Remark = strings.TrimSpace(req.Remark)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 100 {
		return models.Subscription{}, &subscriptionInputError{"订阅名称不能为空且不能超过 100 个字符"}
	}
	if utf8.RuneCountInString(req.Plan) > 100 {
		return models.Subscription{}, &subscriptionInputError{"套餐名称不能超过 100 个字符"}
	}
	if req.Price < 0 || req.Price > 9999999999 {
		return models.Subscription{}, &subscriptionInputError{"套餐费用必须是有效的非负数"}
	}
	if req.Currency == "" || utf8.RuneCountInString(req.Currency) > 8 {
		return models.Subscription{}, &subscriptionInputError{"请输入不超过 8 个字符的货币符号"}
	}
	if req.BillingCycle != -1 && (req.BillingCycle < 1 || req.BillingCycle > 36500) {
		return models.Subscription{}, &subscriptionInputError{"订阅周期必须是 1 至 36500 天，或 -1（一次性）"}
	}
	expiresAt, err := time.ParseInLocation("2006-01-02", req.ExpiresAt, models.GetAppLocation())
	if err != nil {
		return models.Subscription{}, &subscriptionInputError{"请输入有效的到期日期"}
	}
	// started_at is optional: empty clears it, otherwise a date not in the future.
	var startedAt models.LocalTime
	if value := strings.TrimSpace(req.StartedAt); value != "" {
		parsed, err := time.ParseInLocation("2006-01-02", value, models.GetAppLocation())
		if err != nil {
			return models.Subscription{}, &subscriptionInputError{"请输入有效的开通日期"}
		}
		if parsed.After(time.Now()) {
			return models.Subscription{}, &subscriptionInputError{"开通日期不能晚于今天"}
		}
		startedAt = models.FromTime(parsed)
	}
	if req.URL != "" {
		parsed, err := url.ParseRequestURI(req.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return models.Subscription{}, &subscriptionInputError{"订阅地址必须是有效的 HTTP 或 HTTPS 地址"}
		}
	}
	if utf8.RuneCountInString(req.Remark) > 500 {
		return models.Subscription{}, &subscriptionInputError{"备注不能超过 500 个字符"}
	}
	visible := true
	if req.Visible != nil {
		visible = *req.Visible
	}
	showPrice := true
	if req.ShowPrice != nil {
		showPrice = *req.ShowPrice
	}
	return models.Subscription{
		Name:         req.Name,
		Plan:         req.Plan,
		Price:        req.Price,
		Currency:     req.Currency,
		BillingCycle: req.BillingCycle,
		ExpiresAt:    models.FromTime(expiresAt),
		StartedAt:    startedAt,
		AutoRenewal:  req.AutoRenewal,
		Visible:      visible,
		ShowPrice:    showPrice,
		URL:          req.URL,
		Remark:       req.Remark,
	}, nil
}

type subscriptionInputError struct{ message string }

func (e *subscriptionInputError) Error() string { return e.message }

func ListSubscriptions(c *gin.Context) {
	list, err := subscriptions.List(true)
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "获取订阅记录失败："+err.Error())
		return
	}
	api.RespondSuccess(c, list)
}

func AddSubscription(c *gin.Context) {
	var req subscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "请求内容无效："+err.Error())
		return
	}
	item, err := normalizeSubscription(req)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := subscriptions.Create(&item); err != nil {
		api.RespondError(c, http.StatusInternalServerError, "新增订阅失败："+err.Error())
		return
	}
	logSubscriptionAction(c, "create subscription:"+strconv.FormatUint(uint64(item.ID), 10), "info")
	api.RespondSuccess(c, item)
}

func EditSubscription(c *gin.Context) {
	id, ok := parseSubscriptionID(c)
	if !ok {
		return
	}
	var req subscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "请求内容无效："+err.Error())
		return
	}
	input, err := normalizeSubscription(req)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	item, err := subscriptions.Update(id, map[string]interface{}{
		"name": input.Name, "plan": input.Plan, "price": input.Price,
		"currency": input.Currency, "billing_cycle": input.BillingCycle,
		"expires_at": input.ExpiresAt, "started_at": input.StartedAt,
		"auto_renewal": input.AutoRenewal, "visible": input.Visible,
		"show_price": input.ShowPrice, "url": input.URL, "remark": input.Remark,
		"updated_at": models.Now(),
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			api.RespondError(c, http.StatusNotFound, "订阅记录不存在")
		} else {
			api.RespondError(c, http.StatusInternalServerError, "更新订阅失败："+err.Error())
		}
		return
	}
	logSubscriptionAction(c, "update subscription:"+strconv.FormatUint(uint64(id), 10), "info")
	api.RespondSuccess(c, item)
}

func RemoveSubscription(c *gin.Context) {
	id, ok := parseSubscriptionID(c)
	if !ok {
		return
	}
	if err := subscriptions.Delete(id); err != nil {
		if err == gorm.ErrRecordNotFound {
			api.RespondError(c, http.StatusNotFound, "订阅记录不存在")
		} else {
			api.RespondError(c, http.StatusInternalServerError, "删除订阅失败："+err.Error())
		}
		return
	}
	logSubscriptionAction(c, "delete subscription:"+strconv.FormatUint(uint64(id), 10), "warn")
	api.RespondSuccess(c, nil)
}

// OrderSubscriptions saves a manual display order: {"ids": [3, 1, 2]}.
func OrderSubscriptions(c *gin.Context) {
	var req struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "请求内容无效："+err.Error())
		return
	}
	if err := subscriptions.Reorder(req.IDs); err != nil {
		if errors.Is(err, subscriptions.ErrOrderMismatch) {
			api.RespondError(c, http.StatusConflict, "订阅列表已变化，请刷新后重试")
		} else {
			api.RespondError(c, http.StatusInternalServerError, "保存排序失败："+err.Error())
		}
		return
	}
	logSubscriptionAction(c, "order subscriptions", "info")
	api.RespondSuccess(c, nil)
}

func parseSubscriptionID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		api.RespondError(c, http.StatusBadRequest, "无效的订阅记录 ID")
		return 0, false
	}
	return uint(id), true
}

func logSubscriptionAction(c *gin.Context, action, level string) {
	if userUUID, ok := c.Get("uuid"); ok {
		if uuid, ok := userUUID.(string); ok {
			auditlog.Log(c.ClientIP(), uuid, action, level)
		}
	}
}
