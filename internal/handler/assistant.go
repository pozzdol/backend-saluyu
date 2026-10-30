package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

type AssistantHandler struct {
	svc *service.AssistantService
}

func NewAssistantHandler(svc *service.AssistantService) *AssistantHandler {
	return &AssistantHandler{svc: svc}
}

// RegisterAssistantRoutes adds POST /api/v1/assistant/draft. Each call costs an AI request,
// so it has its own limit.
func RegisterAssistantRoutes(r *gin.Engine, h *AssistantHandler, tm *token.Maker) {
	r.POST("/api/v1/assistant/draft", Auth(tm), RateLimit(30, time.Minute), h.Draft)
}

func (h *AssistantHandler) Draft(c *gin.Context) {
	var req model.AssistantRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Messages[len(req.Messages)-1].Role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the last message must be from the user", "code": "validation"})
		return
	}
	reply, err := h.svc.Draft(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "draft with assistant", http.StatusOK, reply, err)
}
