package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/config"
	"github.com/pozzdol/backend-saluyu/internal/database"
	"github.com/pozzdol/backend-saluyu/internal/handler"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

func main() {
	cfg := config.Load()

	db, err := database.NewPostgres(cfg.DBSource)

	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	defer db.Close()

	log.Println("successfully connect to database")

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Printf("listening on port http://localhost%s", cfg.ServerPort)

	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userSvc)

	api := r.Group("/api/v1")
	{
		api.POST("/register", userHandler.Register)
	}

	if err := r.Run(cfg.ServerPort); err != nil {
		log.Fatal("cannot start server:", err)
	}
}
