package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/clients"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/database/records"
	"github.com/komari-monitor/komari/ws"
)

func AddClient(c *gin.Context) {
	var req struct {
		Name  string `json:"name"`
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Preserve the legacy behavior: an empty or invalid body creates an
		// automatically named, ungrouped client.
		req.Name = ""
		req.Group = ""
	}
	req.Group = strings.TrimSpace(req.Group)
	groupExists, err := clients.ServerGroupExists(req.Group)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if !groupExists {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Server group does not exist"})
		return
	}
	uuid, token, err := clients.CreateClientWithNameAndGroup(req.Name, req.Group)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "create client:"+uuid, "info")
	c.JSON(http.StatusOK, gin.H{"status": "success", "uuid": uuid, "token": token, "message": ""})
}

func EditClient(c *gin.Context) {
	var req = make(map[string]interface{})
	uuid := c.Param("uuid")
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": err.Error()})
		return
	}
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid or missing UUID"})
		return
	}
	if rawGroup, exists := req["group"]; exists {
		group, ok := rawGroup.(string)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid group"})
			return
		}
		group = strings.TrimSpace(group)
		exists, err := clients.ServerGroupExists(group)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
			return
		}
		if !exists {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Server group does not exist"})
			return
		}
		req["group"] = group
	}
	if rawPurchasedAt, exists := req["purchased_at"]; exists {
		// Normalize to LocalTime so the stored text is always in the app
		// timezone; an empty value (or null) clears the purchase time.
		value, ok := rawPurchasedAt.(string)
		if rawPurchasedAt != nil && !ok {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid purchased_at"})
			return
		}
		value = strings.TrimSpace(value)
		if value == "" {
			req["purchased_at"] = nil
		} else {
			purchasedAt, err := time.Parse(time.RFC3339, value)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "purchased_at must be an RFC3339 time"})
				return
			}
			if purchasedAt.After(time.Now().Add(time.Minute)) {
				c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "purchased_at cannot be in the future"})
				return
			}
			req["purchased_at"] = models.FromTime(purchasedAt)
		}
	}
	req["uuid"] = uuid
	err := clients.SaveClient(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "edit client:"+uuid, "info")
	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func RemoveClient(c *gin.Context) {
	uuid := c.Param("uuid")
	err := clients.DeleteClient(uuid)
	if err != nil {
		c.JSON(500, gin.H{
			"status": "error",
			"error":  "Failed to delete client" + err.Error(),
		})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "delete client:"+uuid, "warn")
	c.JSON(200, gin.H{"status": "success"})
	ws.DeleteConnectedClients(uuid)
	ws.DeleteLatestReport(uuid)
}

func ClearRecord(c *gin.Context) {
	if err := records.DeleteAll(); err != nil {
		c.JSON(500, gin.H{
			"status":  "error",
			"message": "Failed to delete Record" + err.Error(),
		})
		return
	}
	user_uuid, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), user_uuid.(string), "clear records", "warn")
	c.JSON(200, gin.H{"status": "success"})
}

func GetClient(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid or missing UUID",
		})
		return
	}

	result, err := clients.GetClientByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

func ListClients(c *gin.Context) {
	cls, err := clients.GetAllClientBasicInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, cls)
}

func GetClientToken(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(400, gin.H{
			"status":  "error",
			"message": "Invalid or missing UUID",
		})
		return
	}

	token, err := clients.GetClientTokenByUUID(uuid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "token": token, "message:": ""})
}
