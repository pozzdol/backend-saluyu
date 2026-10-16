package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// Contacts and loans live on LedgerHandler: every loan is a journal entry posted by the ledger.

func (h *LedgerHandler) ListContacts(c *gin.Context) {
	list, err := h.svc.ListContacts(c.Request.Context(), currentClaims(c).UserID)
	if err != nil {
		fail(c, "list contacts", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *LedgerHandler) CreateContact(c *gin.Context) {
	var req model.ContactRequest
	if !bindJSON(c, &req) {
		return
	}
	contact, err := h.svc.CreateContact(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "create contact", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": contact})
}

func (h *LedgerHandler) RenameContact(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.ContactRequest
	if !bindJSON(c, &req) {
		return
	}
	contact, err := h.svc.RenameContact(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "rename contact", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": contact})
}

func (h *LedgerHandler) DeleteContact(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteContact(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete contact", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *LedgerHandler) GetContact(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	d, err := h.svc.ContactDetail(c.Request.Context(), currentClaims(c).UserID, id)
	if err != nil {
		fail(c, "get contact", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}

func (h *LedgerHandler) PostLoan(c *gin.Context) {
	var req model.LoanRequest
	if !bindJSON(c, &req) {
		return
	}
	e, err := h.svc.PostLoan(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "post loan", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": e})
}
