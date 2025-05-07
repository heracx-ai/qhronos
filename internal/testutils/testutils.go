package testutils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// GetEnv returns the value of the environment variable or the default value if not set
func GetEnv(key, defaultValue string) string {
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

// TestRedis returns a Redis client for testing
func TestRedis(t *testing.T) *redis.Client {
	redisHost := GetEnv("TEST_REDIS_HOST", "localhost")
	redisPort := GetEnv("TEST_REDIS_PORT", "6379")
	redisPass := GetEnv("TEST_REDIS_PASS", "")

	client := redis.NewClient(&redis.Options{
		Addr:     redisHost + ":" + redisPort,
		Password: redisPass,
		DB:       0,
	})

	return client
}

func ReadMigrationSQL(t *testing.T) string {
	// Get the absolute path to the migrations directory
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Navigate up to the project root
	projectRoot := filepath.Dir(filepath.Dir(wd))
	migrationPath := filepath.Join(projectRoot, "migrations", "001_initial_schema.sql")

	// Read the migration file
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("failed to read migration file: %v", err)
	}

	return string(content)
}

// GetRedisNamespace returns the Redis namespace prefix for tests
func GetRedisNamespace() string {
	return GetEnv("TEST_REDIS_NAMESPACE", "test:")
}
