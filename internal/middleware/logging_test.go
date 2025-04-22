package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		method         string
		path          string
		body          interface{}
		setupRoute    func(*gin.Engine)
		expectedLevel logrus.Level
		expectedLog   map[string]string
	}{
		{
			name:   "Successful GET request",
			method: "GET",
			path:   "/test",
			setupRoute: func(r *gin.Engine) {
				r.GET("/test", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"message": "success"})
				})
			},
			expectedLevel: logrus.InfoLevel,
			expectedLog: map[string]string{
				"status": "200",
				"method": "GET",
				"path":   "/test",
			},
		},
		{
			name:   "Client error request",
			method: "GET",
			path:   "/not-found",
			setupRoute: func(r *gin.Engine) {
				r.GET("/test", func(c *gin.Context) {
					c.JSON(http.StatusOK, gin.H{"message": "success"})
				})
			},
			expectedLevel: logrus.WarnLevel,
			expectedLog: map[string]string{
				"status": "404",
				"method": "GET",
				"path":   "/not-found",
			},
		},
		{
			name:   "Server error request",
			method: "GET",
			path:   "/error",
			setupRoute: func(r *gin.Engine) {
				r.GET("/error", func(c *gin.Context) {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
				})
			},
			expectedLevel: logrus.ErrorLevel,
			expectedLog: map[string]string{
				"status": "500",
				"method": "GET",
				"path":   "/error",
			},
		},
		{
			name:   "POST request with body",
			method: "POST",
			path:   "/test",
			body:   map[string]string{"key": "value"},
			setupRoute: func(r *gin.Engine) {
				r.POST("/test", func(c *gin.Context) {
					c.JSON(http.StatusCreated, gin.H{"status": "created"})
				})
			},
			expectedLevel: logrus.InfoLevel,
			expectedLog: map[string]string{
				"status": "201",
				"method": "POST",
				"path":   "/test",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			var logBuffer bytes.Buffer

			// Create custom logger
			logger := logrus.New()
			logger.SetOutput(&logBuffer)
			logger.SetFormatter(&logrus.JSONFormatter{})

			// Setup router with logging middleware
			router := gin.New()
			router.Use(Logger(logger))

			// Setup test route
			tt.setupRoute(router)

			// Create request
			var reqBody []byte
			if tt.body != nil {
				reqBody, _ = json.Marshal(tt.body)
			}
			req, _ := http.NewRequest(tt.method, tt.path, bytes.NewBuffer(reqBody))
			req.Header.Set("Content-Type", "application/json")

			// Perform request
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Parse log output
			logEntries := strings.Split(strings.TrimSpace(logBuffer.String()), "\n")
			assert.NotEmpty(t, logEntries, "Expected log output")

			var logEntry map[string]interface{}
			err := json.Unmarshal([]byte(logEntries[0]), &logEntry)
			assert.NoError(t, err, "Failed to parse log entry")

			// Check log level
			assert.Equal(t, tt.expectedLevel.String(), logEntry["level"])

			// Check expected log fields
			for key, expectedValue := range tt.expectedLog {
				assert.Equal(t, expectedValue, logEntry[key], "Unexpected value for log field %s", key)
			}

			// Check additional fields are present
			assert.Contains(t, logEntry, "duration")
			assert.Contains(t, logEntry, "ip")
			assert.Contains(t, logEntry, "user_agent")
		})
	}
} 