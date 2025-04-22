package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHMACService(t *testing.T) {
	defaultSecret := "qhronos.io"
	service := NewHMACService(defaultSecret)

	t.Run("Sign Payload with Default Secret", func(t *testing.T) {
		payload := []byte(`{"event": "test"}`)
		signature := service.SignPayload(payload, "")

		// Verify signature is not empty and has correct length (64 chars for SHA256)
		assert.NotEmpty(t, signature)
		assert.Len(t, signature, 64)

		// Verify signature can be validated
		assert.True(t, service.ValidateSignature(payload, signature, ""))
	})

	t.Run("Sign Payload with Custom Secret", func(t *testing.T) {
		payload := []byte(`{"event": "test"}`)
		customSecret := "custom-secret"
		signature := service.SignPayload(payload, customSecret)

		// Verify signature is not empty and has correct length
		assert.NotEmpty(t, signature)
		assert.Len(t, signature, 64)

		// Verify signature can be validated with custom secret
		assert.True(t, service.ValidateSignature(payload, signature, customSecret))

		// Verify signature fails validation with default secret
		assert.False(t, service.ValidateSignature(payload, signature, ""))
	})

	t.Run("Invalid Signature", func(t *testing.T) {
		payload := []byte(`{"event": "test"}`)
		invalidSignature := "invalid-signature"

		// Verify invalid signature fails validation
		assert.False(t, service.ValidateSignature(payload, invalidSignature, ""))
	})

	t.Run("Different Payloads", func(t *testing.T) {
		payload1 := []byte(`{"event": "test1"}`)
		payload2 := []byte(`{"event": "test2"}`)

		signature1 := service.SignPayload(payload1, "")
		signature2 := service.SignPayload(payload2, "")

		// Verify different payloads produce different signatures
		assert.NotEqual(t, signature1, signature2)

		// Verify signatures validate only with their respective payloads
		assert.True(t, service.ValidateSignature(payload1, signature1, ""))
		assert.True(t, service.ValidateSignature(payload2, signature2, ""))
		assert.False(t, service.ValidateSignature(payload1, signature2, ""))
		assert.False(t, service.ValidateSignature(payload2, signature1, ""))
	})
} 