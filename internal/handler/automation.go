package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/statement"
)

// AutomationHandler serves recurring rules, statement imports and category rules.
type AutomationHandler struct {
	recurring *service.RecurringService
	imports   *service.ImportService
}

func NewAutomationHandler(r *service.RecurringService, i *service.ImportService) *AutomationHandler {
	return &AutomationHandler{recurring: r, imports: i}
}

func (h *AutomationHandler) Rules(c *gin.Context) {
	list, err := h.recurring.Rules(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "list recurring", http.StatusOK, list, err)
}

func (h *AutomationHandler) CreateRule(c *gin.Context) {
	var req model.RecurringRequest
	if !bindJSON(c, &req) {
		return
	}
	rule, err := h.recurring.Save(c.Request.Context(), currentClaims(c).UserID, nil, req)
	respond(c, "create recurring", http.StatusCreated, rule, err)
}

func (h *AutomationHandler) UpdateRule(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.RecurringRequest
	if !bindJSON(c, &req) {
		return
	}
	rule, err := h.recurring.Save(c.Request.Context(), currentClaims(c).UserID, &id, req)
	respond(c, "update recurring", http.StatusOK, rule, err)
}

func (h *AutomationHandler) DeleteRule(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.recurring.Delete(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete recurring", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AutomationHandler) Due(c *gin.Context) {
	list, err := h.recurring.Due(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "due recurring", http.StatusOK, list, err)
}

func (h *AutomationHandler) Confirm(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.ConfirmRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.recurring.Confirm(c.Request.Context(), currentClaims(c).UserID, id, req); err != nil {
		fail(c, "confirm recurring", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AutomationHandler) Skip(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.SkipRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.recurring.Skip(c.Request.Context(), currentClaims(c).UserID, id, req); err != nil {
		fail(c, "skip recurring", err)
		return
	}
	c.Status(http.StatusNoContent)
}

const maxStatement = 5 << 20 // 5 MB: a year of a busy account is well under 1 MB

// Preview reads a multipart upload: file, account_id and an optional mapping (JSON).
func (h *AutomationHandler) Preview(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxStatement+64<<10)
	file, err := c.FormFile("file")
	if err != nil || file.Size > maxStatement {
		c.JSON(http.StatusBadRequest, gin.H{"error": "send a CSV or XLSX file up to 5 MB as \"file\"", "code": "bad_file"})
		return
	}
	accountID, err := uuid.Parse(c.PostForm("account_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_id is required", "code": "validation"})
		return
	}
	var mapping *statement.Mapping
	if raw := c.PostForm("mapping"); raw != "" {
		mapping = &statement.Mapping{}
		if err := json.Unmarshal([]byte(raw), mapping); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "mapping is not valid JSON", "code": "validation"})
			return
		}
	}
	f, err := file.Open()
	if err != nil {
		fail(c, "open upload", err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		fail(c, "read upload", err)
		return
	}
	out, err := h.imports.Preview(c.Request.Context(), currentClaims(c).UserID, accountID, data, mapping)
	respond(c, "preview import", http.StatusOK, out, err)
}

func (h *AutomationHandler) Commit(c *gin.Context) {
	var req model.ImportCommitRequest
	if !bindJSON(c, &req) {
		return
	}
	out, err := h.imports.Commit(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "commit import", http.StatusCreated, out, err)
}

func (h *AutomationHandler) CategoryRules(c *gin.Context) {
	list, err := h.imports.Rules(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "list category rules", http.StatusOK, list, err)
}

func (h *AutomationHandler) CreateCategoryRule(c *gin.Context) {
	var req model.CategoryRuleRequest
	if !bindJSON(c, &req) {
		return
	}
	rule, err := h.imports.CreateRule(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "create category rule", http.StatusCreated, rule, err)
}

func (h *AutomationHandler) DeleteCategoryRule(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.imports.DeleteRule(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete category rule", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func registerAutomationRoutes(app *gin.RouterGroup, h *AutomationHandler) {
	app.GET("/recurring", h.Rules)
	app.POST("/recurring", h.CreateRule)
	app.GET("/recurring/due", h.Due)
	app.PUT("/recurring/:id", h.UpdateRule)
	app.DELETE("/recurring/:id", h.DeleteRule)
	app.POST("/recurring/:id/confirm", h.Confirm)
	app.POST("/recurring/:id/skip", h.Skip)
	app.POST("/imports/preview", RateLimit(30, time.Minute), h.Preview)
	app.POST("/imports/commit", RateLimit(30, time.Minute), h.Commit)
	app.GET("/category-rules", h.CategoryRules)
	app.POST("/category-rules", h.CreateCategoryRule)
	app.DELETE("/category-rules/:id", h.DeleteCategoryRule)
}
