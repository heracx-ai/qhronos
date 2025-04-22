package main

import (
	"fmt"
	"log"

	"github.com/feedloop/qhronos/internal/config"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Create Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Routes
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status": "ok",
		})
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	if err := e.Start(addr); err != nil {
		log.Fatal(err)
	}
} 