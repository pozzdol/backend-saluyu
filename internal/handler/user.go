package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

type UserHandler struct {
	svc *service.UserService
}

func NewUserHandler(svc *service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		fail(c, "register", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": model.ToUserResponse(user)})
}

func (h *UserHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if !bindJSON(c, &req) {
		return
	}

	session, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		fail(c, "login", err)
		return
	}

	setRefreshCookie(c, session)
	c.JSON(http.StatusOK, session)
}

func (h *UserHandler) Refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookie)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}

	session, err := h.svc.Refresh(c.Request.Context(), raw)
	if err != nil {
		clearRefreshCookie(c)
		fail(c, "refresh session", err)
		return
	}

	setRefreshCookie(c, session)
	c.JSON(http.StatusOK, session)
}

func (h *UserHandler) Logout(c *gin.Context) {
	if raw, err := c.Cookie(refreshCookie); err == nil {
		if err := h.svc.Logout(c.Request.Context(), raw); err != nil {
			fail(c, "logout", err)
			return
		}
	}

	clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) Me(c *gin.Context) {
	user, err := h.svc.Me(c.Request.Context(), currentClaims(c))
	if err != nil {
		fail(c, "get current user", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": model.ToUserResponse(user)})
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	var req model.UpdateProfileRequest
	if !bindJSON(c, &req) {
		return
	}

	user, err := h.svc.UpdateProfile(c.Request.Context(), currentClaims(c), req)
	if err != nil {
		fail(c, "update profile", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": model.ToUserResponse(user)})
}

func (h *UserHandler) ChangePassword(c *gin.Context) {
	var req model.ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}

	session, err := h.svc.ChangePassword(c.Request.Context(), currentClaims(c), req)
	if err != nil {
		fail(c, "change password", err)
		return
	}

	// Every other device is logged out; this one keeps working with the new session.
	setRefreshCookie(c, session)
	c.JSON(http.StatusOK, gin.H{"message": "password updated", "access_token": session.AccessToken})
}

// fail maps service errors to HTTP status codes in one place.
// Unknown errors are logged and hidden behind a generic 500.
func fail(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrTokenRevoked),
		errors.Is(err, service.ErrSessionExpired):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrUserInactive):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrWrongPassword):
		// 400, not 401: the token is fine, so the client must not log the user out.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrPasswordTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at most 72 bytes"})
	case errors.Is(err, service.ErrUserNotFound), errors.Is(err, service.ErrAccountNotFound),
		errors.Is(err, service.ErrEntryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrAccountCodeTaken), errors.Is(err, service.ErrNotOpened),
		errors.Is(err, service.ErrBooksNotEmpty), errors.Is(err, service.ErrOpeningLocked):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSameAccount), errors.Is(err, service.ErrWrongAccountType),
		errors.Is(err, service.ErrAccountInactive), errors.Is(err, service.ErrBeforeStart),
		errors.Is(err, service.ErrNotBalanceSheet), errors.Is(err, service.ErrNeedsContact),
		errors.Is(err, service.ErrDuplicateAccount), errors.Is(err, service.ErrWrongCounter),
		errors.Is(err, service.ErrNotExpenseAccount):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		log.Printf("failed to %s: %v", op, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
