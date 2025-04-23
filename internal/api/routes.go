package api

import (
	"github.com/feedloop/qhronos/internal/handlers"
	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// SetupRoutes configures all API routes with their middleware
func SetupRoutes(router *gin.Engine, eventHandler *handlers.EventHandler, occurrenceHandler *handlers.OccurrenceHandler, tokenHandler *handlers.TokenHandler, rateLimiter *middleware.RateLimiter, masterToken string) {
	// Create logger
	logger := logrus.New()

	// Global middleware
	router.Use(middleware.Logger(logger))
	router.Use(middleware.ErrorHandler())

	// Public routes
	public := router.Group("/")
	{
		public.GET("/status", handlers.StatusHandler)
		public.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
	}

	// Protected routes with rate limiting
	protected := router.Group("/")
	protected.Use(middleware.TokenAuth(logger))
	protected.Use(rateLimiter.RateLimit())
	{
		// Event routes
		events := protected.Group("/events")
		{
			events.POST("", eventHandler.CreateEvent)
			events.GET("", eventHandler.ListEventsByTags)
			events.GET("/:id", eventHandler.GetEvent)
			events.PUT("/:id", eventHandler.UpdateEvent)
			events.DELETE("/:id", eventHandler.DeleteEvent)
		}

		// Occurrence routes
		occurrences := protected.Group("/occurrences")
		{
			occurrences.GET("", occurrenceHandler.ListOccurrencesByTags)
			occurrences.GET("/:id", occurrenceHandler.GetOccurrence)
		}

		// Event-specific occurrence routes
		events.GET("/:id/occurrences", occurrenceHandler.ListOccurrencesByEvent)

		// Token routes (requires master token)
		tokens := protected.Group("/tokens")
		tokens.Use(middleware.RequireMasterToken(masterToken))
		{
			tokens.POST("", tokenHandler.CreateToken)
		}
	}
}
