package public

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/subscriptions"
)

func GetSubscriptions(c *gin.Context) {
	list, err := subscriptions.List(false)
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "获取订阅记录失败："+err.Error())
		return
	}
	// Price is omitted server-side when its public visibility is disabled, so
	// it cannot be recovered from the public API response.
	type publicSubscription struct {
		ID           uint             `json:"id"`
		Name         string           `json:"name"`
		Plan         string           `json:"plan"`
		Price        *float64         `json:"price,omitempty"`
		Currency     string           `json:"currency"`
		BillingCycle int              `json:"billing_cycle"`
		ExpiresAt    models.LocalTime `json:"expires_at"`
		StartedAt    models.LocalTime `json:"started_at"`
		AutoRenewal  bool             `json:"auto_renewal"`
		ShowPrice    bool             `json:"show_price"`
		URL          string           `json:"url"`
		Weight       int              `json:"weight"`
	}
	result := make([]publicSubscription, 0, len(list))
	for _, item := range list {
		var price *float64
		if item.ShowPrice {
			price = &item.Price
		}
		result = append(result, publicSubscription{
			ID: item.ID, Name: item.Name, Plan: item.Plan, Price: price,
			Currency: item.Currency, BillingCycle: item.BillingCycle,
			ExpiresAt: item.ExpiresAt, StartedAt: item.StartedAt, AutoRenewal: item.AutoRenewal,
			ShowPrice: item.ShowPrice, URL: item.URL, Weight: item.Weight,
		})
	}
	api.RespondSuccess(c, result)
}
