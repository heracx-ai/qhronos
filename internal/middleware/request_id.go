package middleware

import (
	"github.com/feedloop/qhronos/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const RequestIDKey = "request_id"
const LoggerKey = "logger"

// RequestIDMiddleware injects a request ID into the context and logger for each request
func RequestIDMiddleware(baseLogger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()
		}
		c.Set(RequestIDKey, reqID)
		c.Writer.Header().Set("X-Request-ID", reqID)

		// Attach logger with request ID to context
		logger := logging.WithRequestID(baseLogger, reqID)
		c.Set(LoggerKey, logger)

		c.Next()
	}
}
