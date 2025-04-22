package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter(t *testing.T) {
	// Setup Redis client for testing
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use a different DB for testing
	})
	defer rdb.Close()

	// Clear test database before starting
	rdb.FlushDB(context.Background())

	// Setup test router
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Create rate limiter with small bucket for testing
	rl := NewRateLimiter(rdb,
		WithBucketSize(3),
		WithRefillRate(1),
		WithWindow(1),
	)

	// Add test route with rate limiting
	router.Use(rl.RateLimit())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success"})
	})

	tests := []struct {
		name           string
		expectedStatus int
		headers        map[string]string
		setupTest      func()
	}{
		{
			name:           "First request succeeds",
			expectedStatus: http.StatusOK,
			headers: map[string]string{
				"X-RateLimit-Limit":     "3",
				"X-RateLimit-Remaining": "2",
			},
		},
		{
			name:           "Second request succeeds",
			expectedStatus: http.StatusOK,
			headers: map[string]string{
				"X-RateLimit-Limit":     "3",
				"X-RateLimit-Remaining": "1",
			},
		},
		{
			name:           "Third request succeeds",
			expectedStatus: http.StatusOK,
			headers: map[string]string{
				"X-RateLimit-Limit":     "3",
				"X-RateLimit-Remaining": "0",
			},
		},
		{
			name:           "Fourth request fails",
			expectedStatus: http.StatusTooManyRequests,
			headers: map[string]string{
				"X-RateLimit-Limit":     "3",
				"X-RateLimit-Remaining": "0",
				"Retry-After":           "1.00",
			},
		},
		{
			name:           "Request after refill succeeds",
			expectedStatus: http.StatusOK,
			headers: map[string]string{
				"X-RateLimit-Limit":     "3",
				"X-RateLimit-Remaining": "2",
			},
			setupTest: func() {
				// Wait for token refill
				time.Sleep(1100 * time.Millisecond)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupTest != nil {
				tt.setupTest()
			}

			w := httptest.NewRecorder()
			req, err := http.NewRequest("GET", "/test", nil)
			require.NoError(t, err)

			// Set test client IP
			req.RemoteAddr = "192.168.1.1:12345"

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check rate limit headers
			for header, expected := range tt.headers {
				assert.Equal(t, expected, w.Header().Get(header))
			}

			// Verify reset time is in the future
			resetStr := w.Header().Get("X-RateLimit-Reset")
			reset, err := strconv.ParseInt(resetStr, 10, 64)
			require.NoError(t, err)
			assert.Greater(t, reset, time.Now().Unix())
		})
	}
}

func TestRateLimiterWithToken(t *testing.T) {
	// Setup Redis client for testing
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1,
	})
	defer rdb.Close()

	// Clear test database
	rdb.FlushDB(context.Background())

	// Setup test router with token middleware
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Add middleware to set token ID
	router.Use(func(c *gin.Context) {
		c.Set("token_id", "test_token")
		c.Next()
	})

	// Create rate limiter
	rl := NewRateLimiter(rdb, WithBucketSize(2))
	router.Use(rl.RateLimit())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "success"})
	})

	// Test that rate limit is per token, not IP
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Third request should fail
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.2:12345" // Different IP
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimiterOptions(t *testing.T) {
	rl := NewRateLimiter(nil, // nil client for this test
		WithBucketSize(200),
		WithRefillRate(20),
		WithWindow(2),
	)

	assert.Equal(t, 200, rl.bucketSize)
	assert.Equal(t, 20, rl.refillRate)
	assert.Equal(t, 2, rl.windowInSec)
} 