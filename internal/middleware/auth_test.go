package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestTokenAuth(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Setup a logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Setup tests
	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectAborted  bool
	}{
		{
			name:           "Valid Authorization Header",
			authHeader:     "Bearer test-token",
			expectedStatus: http.StatusOK,
			expectAborted:  false,
		},
		{
			name:           "Missing Authorization Header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectAborted:  true,
		},
		{
			name:           "Invalid Authorization Format",
			authHeader:     "InvalidFormat",
			expectedStatus: http.StatusUnauthorized,
			expectAborted:  true,
		},
		{
			name:           "Bearer With Empty Token",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
			expectAborted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the test route
			router := gin.New()

			// Add a middleware to inject a logger into the context
			router.Use(func(c *gin.Context) {
				c.Set(LoggerKey, logger)
				c.Next()
			})

			// Add the middleware under test
			router.Use(TokenAuth())

			// Add a test handler
			var tokenInContext string

			router.GET("/test", func(c *gin.Context) {
				if token, exists := c.Get("token"); exists {
					tokenInContext = token.(string)
				}
				c.JSON(http.StatusOK, gin.H{"status": "success"})
			})

			// Create test request
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)

			// Set Authorization header if needed
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			// Process request
			router.ServeHTTP(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check if the handler was executed or aborted
			if !tt.expectAborted {
				// Check if token was correctly extracted and stored
				if tt.authHeader != "" {
					expectedToken := tt.authHeader[len("Bearer "):]
					assert.Equal(t, expectedToken, tokenInContext)
				}
			}
		})
	}
}

func TestTokenAuthWithoutLogger(t *testing.T) {
	// Test that the middleware works even when no logger is in the context
	gin.SetMode(gin.TestMode)
	router := gin.New()

	// Use the middleware without setting a logger in context
	router.Use(TokenAuth())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Test valid authorization
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Test missing authorization
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireMasterToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	masterToken := "test-master-token"

	// Create a test router
	router := gin.New()

	// Add middleware
	router.Use(RequireMasterToken(masterToken))

	// Add test handler
	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Test valid master token
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+masterToken)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Test invalid token
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	// Test missing authorization
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// Test incorrect format
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
