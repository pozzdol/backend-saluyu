package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// lang is the request's preferred language for emails: "id" for Indonesian browsers, else "en".
func lang(c *gin.Context) string {
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Accept-Language")), "id") {
		return "id"
	}
	return "en"
}

// accepted is the answer to "send me a link": the same whether or not anything was sent.
func accepted(c *gin.Context) {
	c.JSON(http.StatusAccepted, gin.H{"message": "if the email is registered, a link is on its way"})
}

func (h *UserHandler) ForgotPassword(c *gin.Context) {
	var req model.ForgotPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ForgotPassword(c.Request.Context(), req.Email, lang(c)); err != nil {
		fail(c, "forgot password", err)
		return
	}
	accepted(c)
}

func (h *UserHandler) ResetPassword(c *gin.Context) {
	var req model.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), req); err != nil {
		fail(c, "reset password", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) VerifyEmail(c *gin.Context) {
	var req model.TokenRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		fail(c, "verify email", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) ResendVerification(c *gin.Context) {
	if err := h.svc.ResendVerification(c.Request.Context(), currentClaims(c), lang(c)); err != nil {
		fail(c, "resend verification", err)
		return
	}
	accepted(c)
}

func (h *UserHandler) LoginTOTP(c *gin.Context) {
	var req model.TOTPLoginRequest
	if !bindJSON(c, &req) {
		return
	}
	session, err := h.svc.LoginTOTP(c.Request.Context(), req, c.Request.UserAgent())
	if err != nil {
		fail(c, "login totp", err)
		return
	}
	setRefreshCookie(c, session)
	c.JSON(http.StatusOK, session)
}

// refreshFromCookie is "" when the cookie is missing; the session routes live under the
// cookie's path so it is sent.
func refreshFromCookie(c *gin.Context) string {
	raw, _ := c.Cookie(refreshCookie)
	return raw
}

func (h *UserHandler) ListSessions(c *gin.Context) {
	list, err := h.svc.ListSessions(c.Request.Context(), currentClaims(c), refreshFromCookie(c))
	if err != nil {
		fail(c, "list sessions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list})
}

func (h *UserHandler) EndSession(c *gin.Context) {
	id, ok := paramID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.EndSession(c.Request.Context(), currentClaims(c), id); err != nil {
		fail(c, "end session", err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *UserHandler) RevokeOtherSessions(c *gin.Context) {
	access, err := h.svc.RevokeOtherSessions(c.Request.Context(), currentClaims(c), refreshFromCookie(c))
	if err != nil {
		fail(c, "revoke other sessions", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": access})
}

func (h *UserHandler) SetupTOTP(c *gin.Context) {
	setup, err := h.svc.SetupTOTP(c.Request.Context(), currentClaims(c))
	if err != nil {
		fail(c, "setup totp", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": setup})
}

func (h *UserHandler) EnableTOTP(c *gin.Context) {
	var req model.CodeRequest
	if !bindJSON(c, &req) {
		return
	}
	codes, err := h.svc.EnableTOTP(c.Request.Context(), currentClaims(c), req.Code)
	if err != nil {
		fail(c, "enable totp", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"backup_codes": codes})
}

func (h *UserHandler) DisableTOTP(c *gin.Context) {
	var req model.DisableTOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.DisableTOTP(c.Request.Context(), currentClaims(c), req); err != nil {
		fail(c, "disable totp", err)
		return
	}
	c.Status(http.StatusNoContent)
}
