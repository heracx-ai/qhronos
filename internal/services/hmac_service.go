package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// HMACService handles webhook payload signing
type HMACService struct {
	defaultSecret string
}

// NewHMACService creates a new HMAC signing service
func NewHMACService(defaultSecret string) *HMACService {
	return &HMACService{
		defaultSecret: defaultSecret,
	}
}

// SignPayload signs a webhook payload using HMAC-SHA256
func (s *HMACService) SignPayload(payload []byte, secret string) string {
	if secret == "" {
		secret = s.defaultSecret
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	signature := hex.EncodeToString(h.Sum(nil))
	return signature
}

// ValidateSignature validates a webhook signature
func (s *HMACService) ValidateSignature(payload []byte, signature string, secret string) bool {
	if secret == "" {
		secret = s.defaultSecret
	}

	expectedSignature := s.SignPayload(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
} 