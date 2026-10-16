package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

func RegisterRoutes(r *gin.Engine, users *UserHandler, ledger *LedgerHandler, reports *ReportHandler, tm *token.Maker) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	// Each route gets its own limiter so register traffic cannot lock out login.
	api.POST("/register", RateLimit(10, time.Minute), users.Register)
	api.POST("/login", RateLimit(10, time.Minute), users.Login)

	auth := api.Group("/auth")
	auth.POST("/refresh", RateLimit(60, time.Minute), users.Refresh)
	auth.POST("/logout", users.Logout)

	me := api.Group("/me", Auth(tm))
	me.GET("", users.Me)
	me.PATCH("", users.UpdateProfile)
	me.PUT("/password", RateLimit(10, time.Minute), users.ChangePassword)

	app := api.Group("", Auth(tm))
	app.GET("/accounts", ledger.ListAccounts)
	app.POST("/accounts", ledger.CreateAccount)
	app.PATCH("/accounts/:id", ledger.UpdateAccount)
	app.GET("/accounts/:id/ledger", ledger.Ledger)
	app.GET("/accounts/:id/balance", ledger.Balance)
	app.GET("/opening", ledger.GetOpening)
	app.PUT("/opening", ledger.SetOpening)
	app.POST("/cash-counts", ledger.CashCount)
	app.POST("/reconciliations", ledger.Reconcile)
	app.GET("/entries", ledger.ListEntries)
	app.POST("/entries", ledger.PostEntry)
	app.DELETE("/entries/:id", ledger.DeleteEntry)
	app.GET("/reports/trial-balance", ledger.TrialBalance)
	app.GET("/contacts", ledger.ListContacts)
	app.POST("/contacts", ledger.CreateContact)
	app.GET("/contacts/:id", ledger.GetContact)
	app.PATCH("/contacts/:id", ledger.RenameContact)
	app.DELETE("/contacts/:id", ledger.DeleteContact)
	app.POST("/loans", ledger.PostLoan)
	app.GET("/reports/income-statement", reports.IncomeStatement)
	app.GET("/reports/balance-sheet", reports.BalanceSheet)
	app.GET("/dashboard", reports.Dashboard)
	app.GET("/budgets", reports.Budgets)
	app.PUT("/budgets/:id", reports.SetBudget)
}
