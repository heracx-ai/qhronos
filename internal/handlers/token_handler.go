package handlers

import (
	"net/http"
	"time"

	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type TokenHandler struct {
	tokenService *services.TokenService
}

func NewTokenHandler(tokenService *services.TokenService) *TokenHandler {
	return &TokenHandler{
		tokenService: tokenService,
	}
}

// CreateToken handles the creation of new JWT tokens
func (h *TokenHandler) CreateToken(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	// Get the authorization token from the header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		logger.Warn("Missing authorization header for token creation")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header is required"})
		return
	}

	// Extract the token from the header
	token := authHeader[len("Bearer "):]

	// Validate the master token
	if !h.tokenService.ValidateMasterToken(token) {
		logger.Warn("Invalid master token for token creation")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid master token"})
		return
	}

	// Parse the request body
	var req models.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Invalid token creation request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the request
	if req.Type != models.TokenTypeJWT {
		logger.Warn("Unsupported token type", zap.String("type", string(req.Type)))
		c.JSON(http.StatusBadRequest, gin.H{"error": "only JWT tokens are supported"})
		return
	}

	// Create the JWT token
	tokenString, err := h.tokenService.CreateJWTToken(&req)
	if err != nil {
		logger.Error("Failed to create JWT token", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}
	logger.Info("JWT token created", zap.String("sub", req.Sub), zap.String("access", string(req.Access)))

	// Return the token response
	c.JSON(http.StatusOK, models.CreateTokenResponse{
		Token:     tokenString,
		Type:      req.Type,
		Sub:       req.Sub,
		Access:    req.Access,
		Scope:     req.Scope,
		ExpiresAt: req.ExpiresAt,
	})
}

// AuthMiddleware validates JWT tokens and checks scope/access
func (h *TokenHandler) AuthMiddleware(requiredAccess models.AccessLevel, requiredTags []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the authorization token from the header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header is required"})
			c.Abort()
			return
		}

		// Extract the token from the header
		tokenString := authHeader[len("Bearer "):]

		// Check if it's a master token
		if h.tokenService.ValidateMasterToken(tokenString) {
			c.Next()
			return
		}

		// Validate the JWT token
		token, err := h.tokenService.ValidateJWTToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Check if the token has expired
		if time.Now().After(token.ExpiresAt) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token has expired"})
			c.Abort()
			return
		}

		// Validate access level
		if !h.tokenService.ValidateAccess(token, requiredAccess) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient access level"})
			c.Abort()
			return
		}

		// Validate scope
		if !h.tokenService.ValidateScope(token, requiredTags) {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient scope"})
			c.Abort()
			return
		}

		// Store the token in the context for later use
		c.Set("token", token)
		c.Next()
	}
}
