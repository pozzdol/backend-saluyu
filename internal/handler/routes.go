package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

func RegisterRoutes(r *gin.Engine, users *UserHandler, ledger *LedgerHandler, reports *ReportHandler, share *ShareHandler, credit *CreditHandler, tm *token.Maker) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	// Each route gets its own limiter so register traffic cannot lock out login.
	api.POST("/register", RateLimit(10, time.Minute), users.Register)
	api.POST("/login", RateLimit(10, time.Minute), users.Login)
	api.POST("/login/totp", RateLimit(10, time.Minute), users.LoginTOTP)
	api.POST("/password/forgot", RateLimit(5, time.Minute), users.ForgotPassword)
	api.POST("/password/reset", RateLimit(10, time.Minute), users.ResetPassword)
	api.POST("/email/verify", RateLimit(10, time.Minute), users.VerifyEmail)

	auth := api.Group("/auth")
	auth.POST("/refresh", RateLimit(60, time.Minute), users.Refresh)
	auth.POST("/logout", users.Logout)
	// Under /auth so the refresh cookie comes along and marks the current session.
	auth.GET("/sessions", Auth(tm), users.ListSessions)
	auth.DELETE("/sessions/:id", Auth(tm), users.EndSession)
	auth.POST("/sessions/revoke-others", Auth(tm), users.RevokeOtherSessions)

	me := api.Group("/me", Auth(tm))
	me.GET("", users.Me)
	me.PATCH("", users.UpdateProfile)
	me.PUT("/password", RateLimit(10, time.Minute), users.ChangePassword)
	me.POST("/email/verification", RateLimit(3, time.Minute), users.ResendVerification)
	me.GET("/export", RateLimit(10, time.Minute), reports.Export)
	me.POST("/totp/setup", users.SetupTOTP)
	me.POST("/totp/enable", RateLimit(10, time.Minute), users.EnableTOTP)
	me.POST("/totp/disable", RateLimit(10, time.Minute), users.DisableTOTP)

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
	app.GET("/entries/:id", ledger.GetEntry)
	app.PUT("/entries/:id", ledger.EditEntry)
	app.PUT("/entries/:id/lines", ledger.EditLines)
	app.PUT("/loans/:id", ledger.EditLoan)
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
	registerShareRoutes(app, share)
	registerCreditRoutes(app, credit)
}
