package middleware

import (
	"bytes"
	"io"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// Logger returns a middleware that logs requests using logrus
func Logger(log *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Read the request body
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// Restore the request body for later use
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create a custom response writer to capture the response
		w := &responseWriter{
			ResponseWriter: c.Writer,
			body:          &bytes.Buffer{},
		}
		c.Writer = w

		// Process request
		c.Next()

		// Calculate duration
		duration := time.Since(start)

		// Prepare log fields
		fields := logrus.Fields{
			"status":     strconv.Itoa(c.Writer.Status()),
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"ip":         c.ClientIP(),
			"duration":   duration.String(),
			"user_agent": c.Request.UserAgent(),
		}

		// Add request ID if available
		if requestID := c.GetString("request_id"); requestID != "" {
			fields["request_id"] = requestID
		}

		// Add request body if present and not too large
		if len(requestBody) > 0 && len(requestBody) < 1024 {
			fields["request_body"] = string(requestBody)
		}

		// Add response body if present and not too large
		if w.body.Len() > 0 && w.body.Len() < 1024 {
			fields["response_body"] = w.body.String()
		}

		// Add error if present
		if len(c.Errors) > 0 {
			fields["errors"] = c.Errors.String()
		}

		// Log with appropriate level based on status code
		statusCode := c.Writer.Status()
		switch {
		case statusCode >= 500:
			log.WithFields(fields).Error("Server error")
		case statusCode >= 400:
			log.WithFields(fields).Warn("Client error")
		case statusCode >= 300:
			log.WithFields(fields).Info("Redirection")
		default:
			log.WithFields(fields).Info("Success")
		}
	}
} 