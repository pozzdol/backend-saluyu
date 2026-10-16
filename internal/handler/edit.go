package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func (h *LedgerHandler) PostJournal(c *gin.Context) {
	var req model.LinesRequest
	if !bindJSON(c, &req) {
		return
	}
	e, err := h.svc.PostJournal(c.Request.Context(), currentClaims(c).UserID, req)
	respond(c, "post journal", http.StatusCreated, e, err)
}

func (h *LedgerHandler) Reverse(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.ReverseRequest
	if !bindJSON(c, &req) {
		return
	}
	date := time.Now()
	if req.EntryDate != "" {
		date, _ = time.Parse(time.DateOnly, req.EntryDate) // format checked by binding
	}
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	e, err := h.svc.Reverse(c.Request.Context(), currentClaims(c).UserID, id, date)
	respond(c, "reverse entry", http.StatusCreated, e, err)
}

func (h *LedgerHandler) Goals(c *gin.Context) {
	goals, err := h.svc.Goals(c.Request.Context(), currentClaims(c).UserID)
	respond(c, "list goals", http.StatusOK, goals, err)
}

func (h *LedgerHandler) SaveGoal(c *gin.Context) {
	var id *uuid.UUID
	if c.Param("id") != "" {
		parsed, ok := pathID(c)
		if !ok {
			return
		}
		id = &parsed
	}
	var req model.GoalRequest
	if !bindJSON(c, &req) {
		return
	}
	g, err := h.svc.SaveGoal(c.Request.Context(), currentClaims(c).UserID, id, req)
	status := http.StatusOK
	if id == nil {
		status = http.StatusCreated
	}
	respond(c, "save goal", status, g, err)
}

func (h *LedgerHandler) DeleteGoal(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteGoal(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete goal", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ShareHandler) CreateInviteLink(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	link, err := h.svc.CreateInviteLink(c.Request.Context(), currentClaims(c), id)
	respond(c, "create invite link", http.StatusCreated, link, err)
}

func (h *ShareHandler) AcceptInviteLink(c *gin.Context) {
	var req model.TokenRequest
	if !bindJSON(c, &req) {
		return
	}
	groupID, err := h.svc.AcceptInviteLink(c.Request.Context(), currentClaims(c), req.Token)
	respond(c, "accept invite link", http.StatusOK, gin.H{"group_id": groupID}, err)
}
