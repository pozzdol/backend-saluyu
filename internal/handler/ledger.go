package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

type LedgerHandler struct {
	svc *service.LedgerService
}

func NewLedgerHandler(svc *service.LedgerService) *LedgerHandler {
	return &LedgerHandler{svc: svc}
}

var errBadQuery = errors.New("bad query parameter")

// queryDate reads an optional ?name=YYYY-MM-DD.
func queryDate(c *gin.Context, name string) (*time.Time, error) {
	v := c.Query(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return nil, errBadQuery
	}
	return &t, nil
}

func pathID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return uuid.Nil, false
	}
	return id, true
}

func badQuery(c *gin.Context, name string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameter", "fields": gin.H{name: "is invalid"}})
}

func (h *LedgerHandler) ListAccounts(c *gin.Context) {
	accounts, err := h.svc.ListAccounts(c.Request.Context(), currentClaims(c).UserID)
	if err != nil {
		fail(c, "list accounts", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": accounts})
}

func (h *LedgerHandler) CreateAccount(c *gin.Context) {
	var req model.CreateAccountRequest
	if !bindJSON(c, &req) {
		return
	}
	a, err := h.svc.CreateAccount(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "create account", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": a})
}

func (h *LedgerHandler) UpdateAccount(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req model.UpdateAccountRequest
	if !bindJSON(c, &req) {
		return
	}
	a, err := h.svc.UpdateAccount(c.Request.Context(), currentClaims(c).UserID, id, req)
	if err != nil {
		fail(c, "update account", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": a})
}

func (h *LedgerHandler) Ledger(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	from, err := queryDate(c, "from")
	if err != nil {
		badQuery(c, "from")
		return
	}
	to, err := queryDate(c, "to")
	if err != nil {
		badQuery(c, "to")
		return
	}
	l, err := h.svc.Ledger(c.Request.Context(), currentClaims(c).UserID, id, from, to)
	if err != nil {
		fail(c, "get ledger", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": l})
}

func (h *LedgerHandler) PostEntry(c *gin.Context) {
	var req model.PostEntryRequest
	if !bindJSON(c, &req) {
		return
	}
	e, err := h.svc.PostEntry(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "post entry", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": e})
}

func (h *LedgerHandler) ListEntries(c *gin.Context) {
	f := model.EntryFilter{Query: c.Query("q")}
	var err error
	if f.From, err = queryDate(c, "from"); err != nil {
		badQuery(c, "from")
		return
	}
	if f.To, err = queryDate(c, "to"); err != nil {
		badQuery(c, "to")
		return
	}
	if k := c.Query("kind"); k != "" {
		f.Kind = &k // an unknown kind fails the enum cast in SQL, so validate here first
		switch k {
		case "income", "expense", "transfer", "loan", "repayment", "opening", "cash_count", "adjustment":
		default:
			badQuery(c, "kind")
			return
		}
	}
	if v := c.Query("account_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			badQuery(c, "account_id")
			return
		}
		f.AccountID = &id
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	page = max(page, 1)
	limit = min(max(limit, 1), 200)
	f.Limit, f.Offset = limit, (page-1)*limit

	entries, total, err := h.svc.ListEntries(c.Request.Context(), currentClaims(c).UserID, f)
	if err != nil {
		fail(c, "list entries", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entries, "total": total, "page": page, "limit": limit})
}

func (h *LedgerHandler) DeleteEntry(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteEntry(c.Request.Context(), currentClaims(c).UserID, id); err != nil {
		fail(c, "delete entry", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *LedgerHandler) TrialBalance(c *gin.Context) {
	asOf, err := queryDate(c, "as_of")
	if err != nil {
		badQuery(c, "as_of")
		return
	}
	if asOf == nil {
		// ponytail: server date in UTC; the frontend always sends as_of in the user's local date.
		now := time.Now()
		asOf = &now
	}
	tb, err := h.svc.TrialBalance(c.Request.Context(), currentClaims(c).UserID, *asOf)
	if err != nil {
		fail(c, "trial balance", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": tb})
}

func (h *LedgerHandler) GetOpening(c *gin.Context) {
	o, err := h.svc.GetOpening(c.Request.Context(), currentClaims(c).UserID)
	if err != nil {
		fail(c, "get opening", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": o})
}

func (h *LedgerHandler) SetOpening(c *gin.Context) {
	var req model.OpeningRequest
	if !bindJSON(c, &req) {
		return
	}
	o, err := h.svc.SetOpening(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "set opening", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": o})
}

func (h *LedgerHandler) Balance(c *gin.Context) {
	id, ok := pathID(c)
	if !ok {
		return
	}
	date, err := queryDate(c, "date")
	if err != nil || date == nil {
		badQuery(c, "date")
		return
	}
	b, err := h.svc.Balance(c.Request.Context(), currentClaims(c).UserID, id, *date)
	if err != nil {
		fail(c, "get balance", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"balance": b}})
}

func (h *LedgerHandler) CashCount(c *gin.Context) {
	var req model.ReconcileRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.CashCount(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "cash count", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": res})
}

func (h *LedgerHandler) Reconcile(c *gin.Context) {
	var req model.ReconcileRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.Reconcile(c.Request.Context(), currentClaims(c).UserID, req)
	if err != nil {
		fail(c, "reconcile", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": res})
}
