package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/config"
	"github.com/pozzdol/backend-saluyu/internal/database"
	"github.com/pozzdol/backend-saluyu/internal/handler"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/service"
	"github.com/pozzdol/backend-saluyu/internal/token"
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
	userSvc := service.NewUserService(userRepo, tokenMaker)
	userHandler := handler.NewUserHandler(userSvc)

	r := gin.Default()
	// Trust no proxy headers: otherwise a client can fake X-Forwarded-For and bypass the rate limit.
	// Set the real proxy IPs here when running behind nginx or a load balancer.
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatal("cannot set trusted proxies:", err)
	}
	handler.RegisterRoutes(r, userHandler, tokenMaker)

	log.Printf("listening on http://localhost%s", cfg.ServerPort)
	if err := r.Run(cfg.ServerPort); err != nil {
		log.Fatal("cannot start server:", err)
	}
}
