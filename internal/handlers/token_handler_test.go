package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenHandler(t *testing.T) {
	// Setup
	masterToken := "test-master-token"
	jwtSecret := "test-jwt-secret"
	tokenService := services.NewTokenService(masterToken, jwtSecret)
	handler := NewTokenHandler(tokenService)

	router := gin.Default()
	router.POST("/tokens", handler.CreateToken)

	// Test cases
	t.Run("Create Token - Success", func(t *testing.T) {
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test", "system:test"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tokens", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+masterToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.CreateTokenResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.Token)
		assert.Equal(t, reqBody.Type, response.Type)
		assert.Equal(t, reqBody.Sub, response.Sub)
		assert.Equal(t, reqBody.Access, response.Access)
		assert.Equal(t, reqBody.Scope, response.Scope)
		assert.Equal(t, reqBody.ExpiresAt.Unix(), response.ExpiresAt.Unix())
	})

	t.Run("Create Token - Missing Authorization", func(t *testing.T) {
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tokens", bytes.NewBuffer(body))
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create Token - Invalid Master Token", func(t *testing.T) {
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tokens", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer invalid-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Create Token - Invalid Token Type", func(t *testing.T) {
		reqBody := models.CreateTokenRequest{
			Type:      "invalid-type",
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}

		body, err := json.Marshal(reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tokens", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+masterToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("Create Token - Invalid Request Body", func(t *testing.T) {
		invalidBody := []byte(`{"invalid": "json"`)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/tokens", bytes.NewBuffer(invalidBody))
		req.Header.Set("Authorization", "Bearer "+masterToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestTokenMiddleware(t *testing.T) {
	// Setup
	masterToken := "test-master-token"
	jwtSecret := "test-jwt-secret"
	tokenService := services.NewTokenService(masterToken, jwtSecret)
	handler := NewTokenHandler(tokenService)

	router := gin.Default()
	protectedRoute := router.Group("/protected")
	protectedRoute.Use(handler.AuthMiddleware(models.AccessLevelRead, []string{"user:test"}))

	protectedRoute.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	// Create a valid JWT token for testing
	reqBody := models.CreateTokenRequest{
		Type:      models.TokenTypeJWT,
		Sub:       "test-user",
		Access:    models.AccessLevelRead,
		Scope:     []string{"user:test", "system:test"},
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	jwtToken, err := tokenService.CreateJWTToken(&reqBody)
	require.NoError(t, err)

	t.Run("Middleware - Master Token Success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+masterToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Middleware - JWT Token Success", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+jwtToken)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Middleware - Missing Authorization", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Middleware - Invalid Token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("Middleware - Insufficient Access", func(t *testing.T) {
		// Create a token with insufficient access
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		token, err := tokenService.CreateJWTToken(&reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code) // Should pass as read access is sufficient
	})

	t.Run("Middleware - Insufficient Scope", func(t *testing.T) {
		// Create a token with insufficient scope
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:other"},
			ExpiresAt: time.Now().Add(24 * time.Hour),
		}
		token, err := tokenService.CreateJWTToken(&reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Middleware - Expired Token", func(t *testing.T) {
		// Create an expired token
		reqBody := models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "test-user",
			Access:    models.AccessLevelRead,
			Scope:     []string{"user:test"},
			ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		}
		token, err := tokenService.CreateJWTToken(&reqBody)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
} 