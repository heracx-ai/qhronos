package middleware

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	// RateLimitConfig defines the rate limiting parameters
	defaultBucketSize     = 100  // Maximum number of tokens
	defaultRefillRate     = 10   // Tokens per second
	defaultWindowSeconds  = 1    // Time window in seconds
)

// RateLimiter implements a token bucket algorithm using Redis
type RateLimiter struct {
	rdb         *redis.Client
	bucketSize  int
	refillRate  int
	windowInSec int
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(rdb *redis.Client, opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		rdb:         rdb,
		bucketSize:  defaultBucketSize,
		refillRate:  defaultRefillRate,
		windowInSec: defaultWindowSeconds,
	}

	// Apply options
	for _, opt := range opts {
		opt(rl)
	}

	return rl
}

// RateLimiterOption defines a function to configure RateLimiter
type RateLimiterOption func(*RateLimiter)

// WithBucketSize sets the bucket size
func WithBucketSize(size int) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.bucketSize = size
	}
}

// WithRefillRate sets the refill rate
func WithRefillRate(rate int) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.refillRate = rate
	}
}

// WithWindow sets the time window in seconds
func WithWindow(seconds int) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.windowInSec = seconds
	}
}

// RateLimit returns a middleware that limits request rates using the token bucket algorithm
func (rl *RateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get client identifier (use IP if no token is present)
		clientID := c.ClientIP()
		if token := c.GetString("token_id"); token != "" {
			clientID = token
		}

		key := fmt.Sprintf("rate_limit:%s", clientID)
		now := time.Now().Unix()

		// Use Redis MULTI/EXEC for atomic operations
		pipe := rl.rdb.TxPipeline()

		// Get current bucket state
		bucketKey := fmt.Sprintf("%s:bucket", key)
		lastUpdateKey := fmt.Sprintf("%s:last_update", key)

		// Get current tokens and last update time
		tokens, err := rl.rdb.Get(c, bucketKey).Int()
		if err == redis.Nil {
			tokens = rl.bucketSize // Initialize with full bucket
		} else if err != nil {
			c.Error(err)
			c.AbortWithStatusJSON(500, gin.H{"error": "Rate limit check failed"})
			return
		}

		lastUpdate, err := rl.rdb.Get(c, lastUpdateKey).Int64()
		if err == redis.Nil {
			lastUpdate = now
		} else if err != nil {
			c.Error(err)
			c.AbortWithStatusJSON(500, gin.H{"error": "Rate limit check failed"})
			return
		}

		// Calculate token refill
		elapsed := now - lastUpdate
		refill := int(elapsed) * rl.refillRate
		tokens = min(tokens+refill, rl.bucketSize)

		// Try to consume a token
		if tokens <= 0 {
			retryAfter := float64(1) / float64(rl.refillRate)
			c.Header("X-RateLimit-Limit", strconv.Itoa(rl.bucketSize))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(now+1, 10))
			c.Header("Retry-After", fmt.Sprintf("%.2f", retryAfter))
			c.AbortWithStatusJSON(429, gin.H{"error": "Rate limit exceeded"})
			return
		}

		// Consume token and update bucket
		tokens--
		pipe.Set(c, bucketKey, tokens, time.Duration(rl.windowInSec)*time.Second)
		pipe.Set(c, lastUpdateKey, now, time.Duration(rl.windowInSec)*time.Second)

		if _, err := pipe.Exec(c); err != nil {
			c.Error(err)
			c.AbortWithStatusJSON(500, gin.H{"error": "Rate limit update failed"})
			return
		}

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", strconv.Itoa(rl.bucketSize))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(tokens))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(now+int64(rl.windowInSec), 10))

		c.Next()
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
} 