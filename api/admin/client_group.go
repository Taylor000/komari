package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/api"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/clients"
)

func ListServerGroups(c *gin.Context) {
	groups, err := clients.ListServerGroups()
	if err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to list server groups: "+err.Error())
		return
	}
	api.RespondSuccess(c, groups)
}

func AddServerGroup(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid or missing request body: "+err.Error())
		return
	}
	group, err := clients.CreateServerGroup(req.Name)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	userUUID, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), userUUID.(string), "create server group:"+group.Name, "info")
	api.RespondSuccess(c, group)
}

func EditServerGroup(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		api.RespondError(c, http.StatusBadRequest, "Invalid server group ID")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid or missing request body: "+err.Error())
		return
	}
	group, err := clients.RenameServerGroup(uint(id64), req.Name)
	if err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	userUUID, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), userUUID.(string), "rename server group:"+group.Name, "info")
	api.RespondSuccess(c, group)
}

func RemoveServerGroup(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		api.RespondError(c, http.StatusBadRequest, "Invalid server group ID")
		return
	}
	if err := clients.DeleteServerGroup(uint(id64)); err != nil {
		api.RespondError(c, http.StatusBadRequest, err.Error())
		return
	}
	userUUID, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), userUUID.(string), "delete server group:"+c.Param("id"), "warn")
	api.RespondSuccess(c, nil)
}

func OrderServerGroups(c *gin.Context) {
	var req map[string]int
	if err := c.ShouldBindJSON(&req); err != nil {
		api.RespondError(c, http.StatusBadRequest, "Invalid or missing request body: "+err.Error())
		return
	}
	weights := make(map[uint]int, len(req))
	for rawID, weight := range req {
		id64, err := strconv.ParseUint(rawID, 10, 64)
		if err != nil || id64 == 0 || weight < 0 {
			api.RespondError(c, http.StatusBadRequest, "Invalid server group order")
			return
		}
		weights[uint(id64)] = weight
	}
	if err := clients.OrderServerGroups(weights); err != nil {
		api.RespondError(c, http.StatusInternalServerError, "Failed to order server groups: "+err.Error())
		return
	}
	userUUID, _ := c.Get("uuid")
	auditlog.Log(c.ClientIP(), userUUID.(string), "order server groups", "info")
	api.RespondSuccess(c, nil)
}
