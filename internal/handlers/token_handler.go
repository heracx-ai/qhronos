package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/service"
)

type TokenHandler struct {
	tokenService *service.TokenService
}

func NewTokenHandler(tokenService *service.TokenService) *TokenHandler {
	return &TokenHandler{
		tokenService: tokenService,
	}
}

// CreateToken handles the creation of new JWT tokens
func (h *TokenHandler) CreateToken(c *gin.Context) {
	// Get the authorization token from the header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header is required"})
		return
	}

	// Extract the token from the header
	token := authHeader[len("Bearer "):]

	// Validate the master token
	if !h.tokenService.ValidateMasterToken(token) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid master token"})
		return
	}

	// Parse the request body
	var req models.CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate the request
	if req.Type != models.TokenTypeJWT {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only JWT tokens are supported"})
		return
	}

	// Create the JWT token
	tokenString, err := h.tokenService.CreateJWTToken(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create token"})
		return
	}

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