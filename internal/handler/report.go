package handler

import (
	"bytes"
	"net/http"
	"strconv"
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
	is, err := h.svc.IncomeStatement(c.Request.Context(), currentClaims(c).UserID, date, c.Query("compare") == "1")
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

// Export downloads all of the user's data as a ZIP. It is built in memory first, so a
// failure is a JSON error instead of a broken half-sent file.
// ponytail: in memory; stream it if books grow past tens of megabytes.
func (h *ReportHandler) Export(c *gin.Context) {
	var buf bytes.Buffer
	if err := h.svc.Export(c.Request.Context(), currentClaims(c).UserID, &buf); err != nil {
		fail(c, "export", err)
		return
	}
	name := "saluyu-" + time.Now().Format(time.DateOnly) + ".zip"
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, "application/zip", buf.Bytes())
}

func (h *ReportHandler) Insights(c *gin.Context) {
	if date, ok := reportDate(c); ok {
		v, err := h.svc.Insights(c.Request.Context(), currentClaims(c).UserID, date)
		respond(c, "insights", http.StatusOK, v, err)
	}
}

func (h *ReportHandler) Health(c *gin.Context) {
	if date, ok := reportDate(c); ok {
		v, err := h.svc.Health(c.Request.Context(), currentClaims(c).UserID, date)
		respond(c, "health", http.StatusOK, v, err)
	}
}

func (h *ReportHandler) CashFlow(c *gin.Context) {
	if date, ok := reportDate(c); ok {
		v, err := h.svc.CashFlow(c.Request.Context(), currentClaims(c).UserID, date)
		respond(c, "cash flow", http.StatusOK, v, err)
	}
}

func (h *ReportHandler) Trends(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	n, err := strconv.Atoi(c.DefaultQuery("periods", "12"))
	if err != nil || n < 1 || n > 36 {
		badQuery(c, "periods")
		return
	}
	v, err := h.svc.Trends(c.Request.Context(), currentClaims(c).UserID, date, n)
	respond(c, "trends", http.StatusOK, v, err)
}

func (h *ReportHandler) TaxSummary(c *gin.Context) {
	year, err := strconv.Atoi(c.DefaultQuery("year", strconv.Itoa(time.Now().Year()-1)))
	if err != nil || year < 2000 || year > 2100 {
		badQuery(c, "year")
		return
	}
	v, err := h.svc.TaxSummary(c.Request.Context(), currentClaims(c).UserID, year)
	respond(c, "tax summary", http.StatusOK, v, err)
}

// ExportXLSX downloads one report as an Excel file; built in memory so errors stay JSON.
func (h *ReportHandler) ExportXLSX(c *gin.Context) {
	date, ok := reportDate(c)
	if !ok {
		return
	}
	var buf bytes.Buffer
	name, err := h.svc.ExportReport(c.Request.Context(), currentClaims(c).UserID, c.Query("report"), date, &buf)
	if err != nil {
		fail(c, "export report", err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buf.Bytes())
}
