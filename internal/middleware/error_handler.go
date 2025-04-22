package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorResponse represents the standard error response format
type ErrorResponse struct {
	Error string `json:"error"`
}

// ErrorHandler is a middleware that handles errors in a centralized way
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Check if there are any errors
		if len(c.Errors) > 0 {
			err := c.Errors.Last().Err
			
			// Determine the appropriate status code
			statusCode := http.StatusInternalServerError
			switch err.(type) {
			case *ValidationError:
				statusCode = http.StatusBadRequest
			case *AuthError:
				statusCode = http.StatusUnauthorized
			case *NotFoundError:
				statusCode = http.StatusNotFound
			case *RateLimitError:
				statusCode = http.StatusTooManyRequests
			}

			// Send error response
			c.JSON(statusCode, ErrorResponse{
				Error: err.Error(),
			})
		}
	}
}

// Custom error types for different error scenarios
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

type RateLimitError struct {
	Message string
}

func (e *RateLimitError) Error() string {
	return e.Message
} 