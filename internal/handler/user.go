package handler

import (
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

	user, err := h.svc.Register(c.Request.Context(), req, lang(c))
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

	session, err := h.svc.Login(c.Request.Context(), req, c.Request.UserAgent())
	if err != nil {
		fail(c, "login", err)
		return
	}
	if session.MFARequired { // no session yet: the code comes next, at /login/totp
		c.JSON(http.StatusOK, session)
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

	session, err := h.svc.Refresh(c.Request.Context(), raw, c.Request.UserAgent())
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

	session, err := h.svc.ChangePassword(c.Request.Context(), currentClaims(c), req, c.Request.UserAgent())
	if err != nil {
		fail(c, "change password", err)
		return
	}

	// Every other device is logged out; this one keeps working with the new session.
	setRefreshCookie(c, session)
	c.JSON(http.StatusOK, gin.H{"message": "password updated", "access_token": session.AccessToken})
}

func (h *UserHandler) SetLock(c *gin.Context) {
	var req model.LockRequest
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.svc.SetLock(c.Request.Context(), currentClaims(c), req)
	if err != nil {
		fail(c, "set lock", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": model.ToUserResponse(user)})
}
