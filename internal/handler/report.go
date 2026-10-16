package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

type ReportHandler struct {
	svc *service.ReportService
}

func NewReportHandler(svc *service.ReportService) *ReportHandler {
	return &ReportHandler{svc: svc}
}

// reportDate reads ?date=, defaulting to today. It writes the 400 itself and returns false on a bad date.
func reportDate(c *gin.Context) (time.Time, bool) {
	d, err := queryDate(c, "date")
	if err != nil {
		badQuery(c, "date")
		return time.Time{}, false
	}
	if d != nil {
		return *d, true
	}
	// ponytail: server date in UTC; the frontend always sends ?date= in the user's local date.
	y, m, day := time.Now().UTC().Date()
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC), true
}

func (h *ReportHandler) Dashboard(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	d, err := h.svc.Dashboard(c.Request.Context(), currentClaims(c).UserID, date)
	if err != nil {
		fail(c, "dashboard", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}

func (h *ReportHandler) Budgets(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	b, err := h.svc.Budgets(c.Request.Context(), currentClaims(c).UserID, date)
	if err != nil {
		fail(c, "list budgets", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": b})
}

func (h *ReportHandler) SetBudget(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.BudgetRequest
	if !bindJSON(c, &req) {
		return
	}
	date, _ := time.Parse(time.DateOnly, req.Date) // format already checked by binding
	row, err := h.svc.SetBudget(c.Request.Context(), currentClaims(c).UserID, id, date, req.Amount)
	if err != nil {
		fail(c, "set budget", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

func (h *ReportHandler) IncomeStatement(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	is, err := h.svc.IncomeStatement(c.Request.Context(), currentClaims(c).UserID, date)
	if err != nil {
		fail(c, "income statement", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": is})
}

func (h *ReportHandler) BalanceSheet(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	bs, err := h.svc.BalanceSheet(c.Request.Context(), currentClaims(c).UserID, date)
	if err != nil {
		fail(c, "balance sheet", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": bs})
}
