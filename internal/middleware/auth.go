package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// TokenAuthWithLogger is a middleware that validates JWT tokens with an explicitly provided logger
func TokenAuthWithLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Info("Authorization header is missing")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			logger.Info("Invalid authorization header format",
				zap.String("auth_header", authHeader))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		token := parts[1]
		if token == "" {
			logger.Info("Token is empty")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is required"})
			c.Abort()
			return
		}

		// TODO: Validate JWT token and extract claims
		// For now, we'll just store the token in the context
		c.Set("token", token)
		c.Next()
	}
}

// TokenAuth is a middleware that validates JWT tokens and gets the logger from the context
func TokenAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get logger from context
		logger, exists := c.Get(LoggerKey)
		var zapLogger *zap.Logger
		if exists {
			zapLogger = logger.(*zap.Logger)
		} else {
			// If no logger is in the context, create a no-op logger
			zapLogger = zap.NewNop()
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			zapLogger.Info("Authorization header is missing")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			zapLogger.Info("Invalid authorization header format",
				zap.String("auth_header", authHeader))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		token := parts[1]
		if token == "" {
			zapLogger.Info("Token is empty")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is required"})
			c.Abort()
			return
		}

		// TODO: Validate JWT token and extract claims
		// For now, we'll just store the token in the context
		c.Set("token", token)
		c.Next()
	}
}

// RequireMasterToken is a middleware that requires a master token
func RequireMasterToken(masterToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization header format"})
			c.Abort()
			return
		}

		token := parts[1]
		if token != masterToken {
			c.JSON(http.StatusForbidden, gin.H{"error": "Master token required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
