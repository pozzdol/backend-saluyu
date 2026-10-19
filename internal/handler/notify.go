package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

type NotifyHandler struct {
	svc *service.NotifyService
}

func NewNotifyHandler(svc *service.NotifyService) *NotifyHandler { return &NotifyHandler{svc: svc} }

// RegisterNotifyRoutes adds the email reminder settings under /me.
func RegisterNotifyRoutes(r *gin.Engine, h *NotifyHandler, tm *token.Maker) {
	me := r.Group("/api/v1/me", Auth(tm))
	me.GET("/notifications", h.Get)
	me.PUT("/notifications", h.Set)
}

func (h *NotifyHandler) Get(c *gin.Context) {
	p, err := h.svc.Prefs(c.Request.Context(), currentClaims(c).UserID)
	if err != nil {
		fail(c, "get notification settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

func (h *NotifyHandler) Set(c *gin.Context) {
	var req model.NotifyPrefsRequest
	if !bindJSON(c, &req) {
		return
	}
	p, err := h.svc.SetPrefs(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "set notification settings", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}
