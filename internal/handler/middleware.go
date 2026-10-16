package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

const ctxClaims = "claims"

// Auth rejects requests without a valid "Authorization: Bearer <token>" header
// and stores the token claims in the gin context.
func Auth(tm *token.Maker) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}

		claims, err := tm.Verify(raw)
		if err != nil || claims.Purpose != "" { // an mfa ticket is not an access token
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		c.Set(ctxClaims, claims)
		c.Next()
	}
}

// currentClaims is only valid on routes behind Auth.
func currentClaims(c *gin.Context) *token.Claims {
	return c.MustGet(ctxClaims).(*token.Claims)
}
