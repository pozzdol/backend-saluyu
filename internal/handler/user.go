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

	res, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		fail(c, "login", err)
		return
	}

	c.JSON(http.StatusOK, res)
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

	tok, err := h.svc.ChangePassword(c.Request.Context(), currentClaims(c), req)
	if err != nil {
		fail(c, "change password", err)
		return
	}

	// Old tokens are now revoked; this device keeps working with the new one.
	c.JSON(http.StatusOK, gin.H{"message": "password updated", "token": tok})
}

// fail maps service errors to HTTP status codes in one place.
// Unknown errors are logged and hidden behind a generic 500.
func fail(c *gin.Context, op string, err error) {
	switch {
	case errors.Is(err, service.ErrEmailTaken):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidCredentials), errors.Is(err, service.ErrTokenRevoked):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrUserInactive):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrWrongPassword):
		// 400, not 401: the token is fine, so the client must not log the user out.
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrPasswordTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at most 72 bytes"})
	case errors.Is(err, service.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		log.Printf("failed to %s: %v", op, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
