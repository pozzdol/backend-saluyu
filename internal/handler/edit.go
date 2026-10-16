package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

// editDone answers 200 with the edited entry, or 202 when the edit waits for the other member.
func editDone(c *gin.Context, res *service.EditResult) {
	if res.Pending {
		c.JSON(http.StatusAccepted, gin.H{"status": "pending_approval", "data": res.Entry})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": res.Entry})
}

func (h *LedgerHandler) GetEntry(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	d, err := h.svc.EntryDetail(c.Request.Context(), currentClaims(c).UserID, id)
	if err != nil {
		fail(c, "get entry", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}

func (h *LedgerHandler) EditEntry(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.PostEntryRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.EditEntry(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "edit entry", err)
		return
	}
	editDone(c, res)
}

func (h *LedgerHandler) EditLoan(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.LoanRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.EditLoan(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "edit loan", err)
		return
	}
	editDone(c, res)
}

func (h *LedgerHandler) EditLines(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.LinesRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.EditLines(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "edit lines", err)
		return
	}
	editDone(c, res)
}

func (h *ShareHandler) ApproveChange(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.ApproveChange(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "approve change", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ShareHandler) RejectChange(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RejectChange(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "reject change", err)
		return
	}
	c.Status(http.StatusNoContent)
}
