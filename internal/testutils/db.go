package testutils

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// TestDB creates a test database and returns a connection to it
func TestDB(t testing.TB) *sqlx.DB {
	// Get database connection string from environment variable
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		// Default to Docker container URL
		dbURL = "postgres://postgres:postgres@localhost:5433/qhronos_test?sslmode=disable"
	}

	// Try to connect with retries
	var db *sqlx.DB
	var err error
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		db, err = sqlx.Connect("postgres", dbURL)
		if err == nil {
			break
		}
		fmt.Printf("Failed to connect to database, retrying in 1 second... (attempt %d/%d)\n", i+1, maxRetries)
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("Could not connect to database after %d attempts: %s", maxRetries, err)
	}

	// Check if migrations have already been run
	var count int
	err = db.GetContext(context.Background(), &count, "SELECT COUNT(*) FROM pg_tables WHERE tablename = 'events'")
	if err != nil || count == 0 {
		// Run migrations
		migration, err := os.ReadFile("../../migrations/001_initial_schema.sql")
		if err != nil {
			t.Fatalf("Could not read migration file: %s", err)
		}

		_, err = db.ExecContext(context.Background(), string(migration))
		if err != nil {
			t.Fatalf("Could not run migrations: %s", err)
		}
	}

	return db
}
