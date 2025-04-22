package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		err            error
		expectedStatus int
	}{
		{
			name:           "Validation Error",
			err:            &ValidationError{Message: "invalid input"},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Auth Error",
			err:            &AuthError{Message: "unauthorized"},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Not Found Error",
			err:            &NotFoundError{Message: "resource not found"},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Rate Limit Error",
			err:            &RateLimitError{Message: "too many requests"},
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name:           "Generic Error",
			err:            assert.AnError,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup router
			router := gin.New()
			router.Use(ErrorHandler())

			// Test route that returns an error
			router.GET("/test", func(c *gin.Context) {
				c.Error(tt.err)
			})

			// Make request
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			// Check status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Check response body
			var response ErrorResponse
			err := json.NewDecoder(w.Body).Decode(&response)
			assert.NoError(t, err)
			assert.Equal(t, tt.err.Error(), response.Error)
		})
	}
} 