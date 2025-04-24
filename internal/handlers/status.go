package handlers

import (
	"net/http"
	"time"

	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var startTime = time.Now()

// getStartTime returns the start time of the application
func getStartTime() time.Time {
	return startTime
}

// StatusResponse represents the status endpoint response
type StatusResponse struct {
	Status        string   `json:"status"`
	UptimeSeconds int64    `json:"uptime_seconds"`
	Version       string   `json:"version"`
	JWT           JWTInfo  `json:"jwt"`
	HMAC          HMACInfo `json:"hmac"`
}

// JWTInfo contains JWT configuration information
type JWTInfo struct {
	Issuer            string `json:"issuer"`
	Algorithm         string `json:"algorithm"`
	DefaultExpSeconds int    `json:"default_exp_seconds"`
	TokenInfo         struct {
		Sub    string   `json:"sub"`
		Access string   `json:"access"`
		Scope  []string `json:"scope"`
		Exp    int64    `json:"exp"`
	} `json:"token_info"`
}

// HMACInfo contains HMAC signing configuration
type HMACInfo struct {
	Algorithm              string `json:"algorithm"`
	SignatureHeader        string `json:"signature_header"`
	DefaultSecret          string `json:"default_secret"`
	EventOverrideSupported bool   `json:"event_override_supported"`
}

// StatusHandler handles the status endpoint
func StatusHandler(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	response := StatusResponse{
		Status:        "ok",
		UptimeSeconds: int64(time.Since(getStartTime()).Seconds()),
		Version:       "1.0.0",
		JWT: JWTInfo{
			Issuer:            "qhronos",
			Algorithm:         "HS256",
			DefaultExpSeconds: 3600,
			TokenInfo: struct {
				Sub    string   `json:"sub"`
				Access string   `json:"access"`
				Scope  []string `json:"scope"`
				Exp    int64    `json:"exp"`
			}{
				Sub:    "rizqme",
				Access: "read_write",
				Scope:  []string{"user:rizqme", "system:qa"},
				Exp:    1714569600,
			},
		},
		HMAC: HMACInfo{
			Algorithm:              "HMAC-SHA256",
			SignatureHeader:        "X-Qhronos-Signature",
			DefaultSecret:          "qhronos.io",
			EventOverrideSupported: true,
		},
	}
	logger.Info("Status endpoint checked", zap.Int64("uptime_seconds", response.UptimeSeconds))
	c.JSON(http.StatusOK, response)
}
