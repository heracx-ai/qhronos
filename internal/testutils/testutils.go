package testutils

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/database"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

var (
	testDB     *sqlx.DB
	dbInitOnce sync.Once
)

// TestDB returns a new test database connection
func TestDB(t *testing.T) *sqlx.DB {
	t.Helper()

	var initErr error
	dbInitOnce.Do(func() {
		cfg := database.Config{
			Host:     getEnv("TEST_DB_HOST", "localhost"),
			Port:     5433,
			User:     getEnv("TEST_DB_USER", "postgres"),
			Password: getEnv("TEST_DB_PASSWORD", "postgres"),
			DBName:   getEnv("TEST_DB_NAME", "postgres_test"),
			SSLMode:  getEnv("TEST_DB_SSL_MODE", "disable"),
		}

		testDB, initErr = database.NewPostgresDB(cfg)
		if initErr != nil {
			return
		}

		// Clean up any existing data
		_, initErr = testDB.Exec("TRUNCATE TABLE events, occurrences CASCADE")
	})

	if initErr != nil {
		t.Fatalf("Failed to initialize test database: %v", initErr)
	}

	t.Cleanup(func() {
		// Clean up test data
		_, err := testDB.Exec("TRUNCATE TABLE events, occurrences CASCADE")
		if err != nil {
			t.Errorf("Failed to clean up test data: %v", err)
		}
	})

	return testDB
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// RandomUUID returns a new random UUID for testing
func RandomUUID() uuid.UUID {
	return uuid.New()
}

// RandomTime returns a random time for testing
func RandomTime() time.Time {
	return time.Now().Add(time.Duration(uuid.New().ID()) * time.Hour)
} 