package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

func RegisterRoutes(r *gin.Engine, users *UserHandler, tm *token.Maker) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	// Each route gets its own limiter so register traffic cannot lock out login.
	api.POST("/register", RateLimit(10, time.Minute), users.Register)
	api.POST("/login", RateLimit(10, time.Minute), users.Login)

	me := api.Group("/me", Auth(tm))
	me.GET("", users.Me)
	me.PATCH("", users.UpdateProfile)
	me.PUT("/password", RateLimit(10, time.Minute), users.ChangePassword)
}
