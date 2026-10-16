package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

type CreditHandler struct {
	svc *service.CreditService
}

func NewCreditHandler(svc *service.CreditService) *CreditHandler {
	return &CreditHandler{svc: svc}
}

// respond writes {"data": v} or the mapped error.
func respond(c *gin.Context, op string, status int, v any, err error) {
	if err != nil {
		fail(c, op, err)
		return
	}
	c.JSON(status, gin.H{"data": v})
}

func (h *CreditHandler) CreateCard(c *gin.Context) {
	var req model.CardRequest
	if !bindJSON(c, &req) {
		return
	}
	card, err := h.svc.CreateCard(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "create card", http.StatusCreated, card, err)
}

func (h *CreditHandler) UpdateCard(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.CardSettings
	if !bindJSON(c, &req) {
		return
	}
	card, err := h.svc.UpdateCard(c.Request.Context(), currentClaims(c).UserID, id, req)
	respond(c, "update card", http.StatusOK, card, err)
}

func (h *CreditHandler) Cards(c *gin.Context) {
	cards, err := h.svc.Cards(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "list cards", http.StatusOK, cards, err)
}

func (h *CreditHandler) Card(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	card, err := h.svc.Card(c.Request.Context(), currentClaims(c).UserID, id)
	respond(c, "get card", http.StatusOK, card, err)
}

func (h *CreditHandler) CreatePlan(c *gin.Context) {
	var req model.PlanRequest
	if !bindJSON(c, &req) {
		return
	}
	plan, err := h.svc.CreatePlan(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "create installment plan", http.StatusCreated, plan, err)
}

func (h *CreditHandler) Plans(c *gin.Context) {
	plans, err := h.svc.Plans(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "list installment plans", http.StatusOK, plans, err)
}

func (h *CreditHandler) Plan(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	plan, err := h.svc.Plan(c.Request.Context(), currentClaims(c).UserID, id)
	respond(c, "get installment plan", http.StatusOK, plan, err)
}

func (h *CreditHandler) Pay(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.PayInstallmentRequest
	if !bindJSON(c, &req) {
		return
	}
	plan, err := h.svc.PayInstallment(c.Request.Context(), currentClaims(c).UserID, id, req)
	respond(c, "pay installment", http.StatusCreated, plan, err)
}

func (h *CreditHandler) DeletePlan(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeletePlan(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete installment plan", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func registerCreditRoutes(app *gin.RouterGroup, h *CreditHandler) {
	app.GET("/cards", h.Cards)
	app.POST("/cards", h.CreateCard)
	app.GET("/cards/:id", h.Card)
	app.PATCH("/cards/:id", h.UpdateCard)
	app.GET("/installments", h.Plans)
	app.POST("/installments", h.CreatePlan)
	app.GET("/installments/:id", h.Plan)
	app.POST("/installments/:id/pay", h.Pay)
	app.DELETE("/installments/:id", h.DeletePlan)
}
