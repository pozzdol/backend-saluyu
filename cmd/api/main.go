package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/config"
	"github.com/pozzdol/backend-saluyu/internal/database"
	"github.com/pozzdol/backend-saluyu/internal/handler"
	"github.com/pozzdol/backend-saluyu/internal/mail"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
	"github.com/pozzdol/backend-saluyu/internal/totp"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgres(cfg.DBSource)
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	defer db.Close()

	log.Println("successfully connect to database")

	tokenMaker := token.NewMaker(cfg.JWTSecret, cfg.JWTDuration)

	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	userSvc := service.NewUserService(service.UserDeps{
		Users:      userRepo,
		Sessions:   sessionRepo,
		Security:   repository.NewSecurityRepository(db),
		Mail:       mail.New(cfg.ResendAPIKey, cfg.MailFrom),
		Token:      tokenMaker,
		TOTP:       totp.NewBox(cfg.JWTSecret),
		RefreshTTL: cfg.RefreshDuration,
		AppURL:     cfg.AppURL,
	})
	userHandler := handler.NewUserHandler(userSvc)
	ledgerRepo := repository.NewLedgerRepository(db)
	ledgerSvc := service.NewLedgerService(ledgerRepo)
	reportSvc := service.NewReportService(ledgerRepo)
	ledgerHandler := handler.NewLedgerHandler(ledgerSvc)
	reportHandler := handler.NewReportHandler(reportSvc)
	shareHandler := handler.NewShareHandler(service.NewShareService(ledgerRepo, ledgerSvc, reportSvc))

	r := gin.Default()
	// Trust no proxy headers: otherwise a client can fake X-Forwarded-For and bypass the rate limit.
	// Set the real proxy IPs here when running behind nginx or a load balancer.
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatal("cannot set trusted proxies:", err)
	}
	creditHandler := handler.NewCreditHandler(service.NewCreditService(ledgerRepo, ledgerSvc))
	recurringSvc := service.NewRecurringService(ledgerRepo, ledgerSvc)
	reportSvc.UseRecurring(recurringSvc)
	recurringSvc.StartJob(context.Background(), time.Hour)
	automationHandler := handler.NewAutomationHandler(recurringSvc, service.NewImportService(ledgerRepo, ledgerSvc))
	handler.RegisterRoutes(r, userHandler, ledgerHandler, reportHandler, shareHandler, creditHandler, automationHandler, tokenMaker)

	log.Printf("listening on http://localhost%s", cfg.ServerPort)
	if err := r.Run(cfg.ServerPort); err != nil {
		log.Fatal("cannot start server:", err)
	}
}
