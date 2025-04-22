package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestStatusHandler(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/status", StatusHandler)

	// Test
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/status", nil)
	router.ServeHTTP(w, req)

	// Assert status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var response StatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Assert response fields
	assert.Equal(t, "ok", response.Status)
	assert.Equal(t, "1.0.0", response.Version)
	assert.GreaterOrEqual(t, response.UptimeSeconds, int64(0))

	// Assert JWT info
	assert.Equal(t, "qhronos", response.JWT.Issuer)
	assert.Equal(t, "HS256", response.JWT.Algorithm)
	assert.Equal(t, 3600, response.JWT.DefaultExpSeconds)
	assert.Equal(t, "rizqme", response.JWT.TokenInfo.Sub)
	assert.Equal(t, "read_write", response.JWT.TokenInfo.Access)
	assert.Equal(t, []string{"user:rizqme", "system:qa"}, response.JWT.TokenInfo.Scope)
	assert.Equal(t, int64(1714569600), response.JWT.TokenInfo.Exp)

	// Assert HMAC info
	assert.Equal(t, "HMAC-SHA256", response.HMAC.Algorithm)
	assert.Equal(t, "X-Qhronos-Signature", response.HMAC.SignatureHeader)
	assert.Equal(t, "qhronos.io", response.HMAC.DefaultSecret)
	assert.True(t, response.HMAC.EventOverrideSupported)
}

func TestStatusHandler_Uptime(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/status", StatusHandler)

	// Record start time
	start := time.Now()

	// Test
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/status", nil)
	router.ServeHTTP(w, req)

	// Assert status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse response
	var response StatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	// Assert uptime is within expected range
	elapsed := time.Since(start).Seconds()
	assert.GreaterOrEqual(t, response.UptimeSeconds, int64(elapsed))
	assert.Less(t, response.UptimeSeconds, int64(elapsed+1)) // Should be within 1 second
} 