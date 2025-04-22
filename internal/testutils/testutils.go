package testutils

import (
	"os"
	"time"

	"github.com/google/uuid"
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