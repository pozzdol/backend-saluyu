package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

const (
	refreshCookie = "refresh_token"
	// The browser sends the cookie only to /auth/refresh and /auth/logout, never to other API calls.
	refreshPath = "/api/v1/auth"
)

// The refresh token lives in a cookie that JavaScript cannot read (HttpOnly), that only travels
// over HTTPS (Secure; browsers allow it on http://localhost for development) and that other sites
// cannot make the browser send (SameSite=Strict, which also blocks CSRF on refresh and logout).
func setRefreshCookie(c *gin.Context, s *model.Session) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookie,
		Value:    s.RefreshToken,
		Path:     refreshPath,
		Expires:  s.RefreshExpires,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     refreshCookie,
		Path:     refreshPath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}
