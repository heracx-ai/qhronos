package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/feedloop/qhronos/internal/api"
	"github.com/feedloop/qhronos/internal/config"
	"github.com/feedloop/qhronos/internal/database"
	"github.com/feedloop/qhronos/internal/handlers"
	"github.com/feedloop/qhronos/internal/logging"
	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/scheduler"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

// NOTE: At least one .sql file must exist in migrations/ for embedding to work.
// Make sure to build from the project root so the path is correct.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

func runMigrations(cfg *config.Config) error {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", d, "postgres", driver)
	if err != nil {
		return err
	}
	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		return err
	}
	fmt.Println("Migrations applied successfully.")
	return nil
}

func main() {
	// CLI flags
	configPath := pflag.StringP("config", "c", "config.yaml", "Path to config file")
	migrate := pflag.BoolP("migrate", "m", false, "Run database migrations and exit")
	version := pflag.BoolP("version", "v", false, "Print version and exit")
	port := pflag.IntP("port", "p", 8080, "HTTP server listen port")
	logLevel := pflag.StringP("log-level", "l", "info", "Log level (debug, info, warn, error)")
	masterToken := pflag.String("master-token", "", "Override master token from config")
	jwtSecret := pflag.String("jwt-secret", "", "Override JWT secret from config")

	pflag.Parse()

	if *version {
		fmt.Println("qhronosd version 1.0.0")
		os.Exit(0)
	}

	if *migrate {
		cfg, err := config.LoadWithPath(*configPath)
		if err != nil {
			panic("Failed to load configuration: " + err.Error())
		}
		err = runMigrations(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadWithPath(*configPath)
	if err != nil {
		panic("Failed to load configuration: " + err.Error())
	}

	// Override config with CLI flags if set
	if pflag.Lookup("port").Changed {
		cfg.Server.Port = *port
	}
	if pflag.Lookup("log-level").Changed {
		cfg.Logging.Level = *logLevel
	}
	if pflag.Lookup("master-token").Changed && *masterToken != "" {
		cfg.Auth.MasterToken = *masterToken
	}
	if pflag.Lookup("jwt-secret").Changed && *jwtSecret != "" {
		cfg.Auth.JWTSecret = *jwtSecret
	}

	// Initialize logger
	logger, err := logging.InitLogger(logging.LoggingConfig(cfg.Logging))
	if err != nil {
		panic("Failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	logger.Info("Configuration loaded", zap.Any("config", cfg))

	// After loading cfg and before calling StartArchivalScheduler
	durations, err := cfg.ParseRetentionDurations()
	if err != nil {
		logger.Fatal("Failed to parse retention durations", zap.Error(err))
	}

	// Initialize database connection
	db, err := database.Connect(cfg.Database.ToDBConfig())
	if err != nil {
		logger.Fatal("Failed to connect to database", zap.Error(err))
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
	eventRepo := repository.NewEventRepository(db, logger, redisClient)
	occurrenceRepo := repository.NewOccurrenceRepository(db, logger)

	// Initialize scheduler services
	schedulerService := scheduler.NewScheduler(redisClient, logger)
	expander := scheduler.NewExpander(
		schedulerService,
		eventRepo,
		occurrenceRepo,
		cfg.Scheduler.LookAheadDuration,
		cfg.Scheduler.ExpansionInterval,
		logger,
	)

	// Initialize services
	tokenService := services.NewTokenService(cfg.Auth.MasterToken, cfg.Auth.JWTSecret)
	hmacService := services.NewHMACService(cfg.HMAC.DefaultSecret)
	dispatcher := scheduler.NewDispatcher(eventRepo, occurrenceRepo, hmacService, logger)

	// Initialize handlers
	eventHandler := handlers.NewEventHandler(eventRepo)
	occurrenceHandler := handlers.NewOccurrenceHandler(eventRepo, occurrenceRepo)
	tokenHandler := handlers.NewTokenHandler(tokenService)

	// Initialize middleware
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// Initialize router
	router := gin.Default()

	// Register request ID middleware
	router.Use(middleware.RequestIDMiddleware(logger))

	// Setup routes with middleware
	api.SetupRoutes(router, eventHandler, occurrenceHandler, tokenHandler, rateLimiter, cfg.Auth.MasterToken)

	// Start HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// Start archival scheduler in background
	archivalStopCh := make(chan struct{})
	scheduler.StartArchivalScheduler(db, cfg.Archival.CheckPeriod, *durations, archivalStopCh, logger)

	// Start scheduler services in background
	ctx := context.Background()
	go func() {
		if err := expander.Run(ctx); err != nil {
			logger.Error("Expander error", zap.Error(err))
		}
	}()

	go func() {
		if err := dispatcher.Run(ctx, schedulerService); err != nil {
			logger.Error("Dispatcher error", zap.Error(err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		logger.Info("Shutting down server...")

		// Stop archival scheduler
		close(archivalStopCh)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Fatal("Server forced to shutdown", zap.Error(err))
		}
	}()

	logger.Info("Starting server", zap.Int("port", cfg.Server.Port))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server error", zap.Error(err))
	}
}
