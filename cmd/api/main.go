package main

import (
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/feedloop/qhronos/internal/database"
	"github.com/feedloop/qhronos/internal/handlers"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/service"
)

func main() {
	// Initialize database connection
	db, err := database.NewPostgresDB(database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     5433,
		User:     getEnv("DB_USER", "postgres"),
		Password: getEnv("DB_PASSWORD", "postgres"),
		DBName:   getEnv("DB_NAME", "postgres"),
		SSLMode:  getEnv("DB_SSL_MODE", "disable"),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize repositories
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)

	// Initialize services
	tokenService := service.NewTokenService(
		getEnv("MASTER_TOKEN", "your-master-token-here"),
		getEnv("JWT_SECRET", "your-jwt-secret-here"),
	)

	// Initialize handlers
	eventHandler := handlers.NewEventHandler(eventRepo)
	occurrenceHandler := handlers.NewOccurrenceHandler(occurrenceRepo)
	tokenHandler := handlers.NewTokenHandler(tokenService)

	// Initialize router
	r := gin.Default()

	// Token routes
	tokens := r.Group("/tokens")
	{
		tokens.POST("", tokenHandler.CreateToken)
	}

	// Event routes
	events := r.Group("/events")
	{
		events.POST("", eventHandler.CreateEvent)
		events.GET("/:id", eventHandler.GetEvent)
		events.PUT("/:id", eventHandler.UpdateEvent)
		events.DELETE("/:id", eventHandler.DeleteEvent)
		events.GET("", eventHandler.ListEventsByTags)
	}

	// Occurrence routes
	occurrences := r.Group("/occurrences")
	{
		occurrences.GET("/:id", occurrenceHandler.GetOccurrence)
		occurrences.GET("", occurrenceHandler.ListOccurrencesByTags)
	}

	// Start server
	port := getEnv("PORT", "8080")
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
} 