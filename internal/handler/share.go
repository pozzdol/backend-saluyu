package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

type ShareHandler struct {
	svc *service.ShareService
}

func NewShareHandler(svc *service.ShareService) *ShareHandler {
	return &ShareHandler{svc: svc}
}

// paramID parses a UUID path parameter; a malformed one is just "not found".
func paramID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return uuid.Nil, false
	}
	return id, true
}

func (h *ShareHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroups(c.Request.Context(), currentClaims(c).UserID)
	if err != nil {
		fail(c, "list groups", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": groups})
}

func (h *ShareHandler) CreateGroup(c *gin.Context) {
	var req model.GroupRequest
	if !bindJSON(c, &req) {
		return
	}
	g, err := h.svc.CreateGroup(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "create group", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": g})
}

func (h *ShareHandler) Invite(c *gin.Context) {
	groupID, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.InviteRequest
	if !bindJSON(c, &req) {
		return
	}
	inv, err := h.svc.Invite(c.Request.Context(), currentClaims(c).UserID, groupID, req)
	if err != nil {
		fail(c, "invite", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": inv})
}

func (h *ShareHandler) CancelInvite(c *gin.Context) {
	groupID, ok := paramID(c, "id")
	if !ok {
		return
	}
	inviteID, ok := paramID(c, "invite_id")
	if !ok {
		return
	}
	if err := h.svc.CancelInvite(c.Request.Context(), currentClaims(c).UserID, groupID, inviteID); err != nil {
		fail(c, "cancel invite", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ShareHandler) Leave(c *gin.Context) {
	groupID, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.LeaveGroup(c.Request.Context(), currentClaims(c).UserID, groupID); err != nil {
		fail(c, "leave group", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ShareHandler) Summary(c *gin.Context) {
	groupID, ok := paramID(c, "id")
	if !ok {
		return
	}
	date, ok := reportDate(c)
	if !ok {
		return
	}
	s, err := h.svc.Summary(c.Request.Context(), currentClaims(c).UserID, groupID, date)
	if err != nil {
		fail(c, "group summary", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

func (h *ShareHandler) Inbox(c *gin.Context) {
	inbox, err := h.svc.Inbox(c.Request.Context(), currentClaims(c))
	if err != nil {
		fail(c, "inbox", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": inbox})
}

func (h *ShareHandler) respond(accept bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := paramID(c, "id")
		if !ok {
			return
		}
		if err := h.svc.RespondInvite(c.Request.Context(), currentClaims(c), id, accept); err != nil {
			fail(c, "respond to invite", err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func (h *ShareHandler) LinkContact(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.LinkContactRequest
	if !bindJSON(c, &req) {
		return
	}
	contact, err := h.svc.LinkContact(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "link contact", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": contact})
}

func (h *ShareHandler) ApproveLoan(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	var req model.ApproveLoanRequest
	if !bindJSON(c, &req) {
		return
	}
	entryID, err := h.svc.Approve(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "approve loan", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"to_entry_id": entryID}})
}

func (h *ShareHandler) RejectLoan(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Reject(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "reject loan", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// registerShareRoutes keeps phase 5 routes together. Invites are rate limited per hour:
// each one stores an email, so this stops the endpoint becoming a spam list.
func registerShareRoutes(app *gin.RouterGroup, h *ShareHandler) {
	app.GET("/groups", h.ListGroups)
	app.POST("/groups", h.CreateGroup)
	app.POST("/groups/:id/invites", RateLimit(20, time.Hour), h.Invite)
	app.DELETE("/groups/:id/invites/:invite_id", h.CancelInvite)
	app.POST("/groups/:id/leave", h.Leave)
	app.GET("/groups/:id/summary", h.Summary)
	app.GET("/inbox", h.Inbox)
	app.POST("/invites/:id/accept", h.respond(true))
	app.POST("/invites/:id/reject", h.respond(false))
	app.PUT("/contacts/:id/link", h.LinkContact)
	app.POST("/loan-requests/:id/approve", h.ApproveLoan)
	app.POST("/loan-requests/:id/reject", h.RejectLoan)
	app.POST("/loan-change-requests/:id/approve", h.ApproveChange)
	app.POST("/loan-change-requests/:id/reject", h.RejectChange)
}
