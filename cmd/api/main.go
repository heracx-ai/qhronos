package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/feedloop/qhronos/internal/api"
	"github.com/feedloop/qhronos/internal/config"
	"github.com/feedloop/qhronos/internal/database"
	"github.com/feedloop/qhronos/internal/handlers"
	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/feedloop/qhronos/internal/services/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database connection
	db, err := database.Connect(cfg.Database.ToDBConfig())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer redisClient.Close()

	// Initialize repositories
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)

	// Initialize scheduler services
	schedulerService := scheduler.NewScheduler(redisClient)

	// Initialize services
	tokenService := services.NewTokenService(cfg.Auth.MasterToken, cfg.Auth.JWTSecret)
	hmacService := services.NewHMACService(cfg.HMAC.DefaultSecret)
	dispatcher := scheduler.NewDispatcher(eventRepo, occurrenceRepo, hmacService)

	// Initialize handlers
	eventHandler := handlers.NewEventHandler(eventRepo)
	occurrenceHandler := handlers.NewOccurrenceHandler(eventRepo, occurrenceRepo)
	tokenHandler := handlers.NewTokenHandler(tokenService)

	// Initialize middleware
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// Initialize router
	router := gin.Default()

	// Setup routes with middleware
	api.SetupRoutes(router, eventHandler, occurrenceHandler, tokenHandler, rateLimiter, cfg.Auth.MasterToken)

	// Start HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start scheduler and dispatcher in background
	ctx := context.Background()
	go func() {
		if err := dispatcher.Run(ctx, schedulerService); err != nil {
			log.Printf("Dispatcher error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Fatal("Server forced to shutdown:", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Server error:", err)
	}
} 