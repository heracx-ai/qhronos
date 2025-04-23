package services

import (
	"testing"
	"time"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestTokenService_CreateAndValidateJWT(t *testing.T) {
	masterToken := "test-master-token"
	secret := "test-secret"
	ts := NewTokenService(masterToken, secret)

	t.Run("valid token", func(t *testing.T) {
		req := &models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "user1",
			Access:    models.AccessLevelRead,
			Scope:     []string{"scope1", "scope2"},
			ExpiresAt: time.Now().Add(time.Minute),
		}
		token, err := ts.CreateJWTToken(req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := ts.ValidateJWTToken(token)
		assert.NoError(t, err)
		assert.Equal(t, req.Sub, claims.Sub)
		assert.Equal(t, req.Access, claims.Access)
		assert.ElementsMatch(t, req.Scope, claims.Scope)
	})

	t.Run("expired token", func(t *testing.T) {
		req := &models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "user2",
			Access:    models.AccessLevelRead,
			Scope:     []string{"scope1"},
			ExpiresAt: time.Now().Add(-time.Minute),
		}
		token, err := ts.CreateJWTToken(req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		_, err = ts.ValidateJWTToken(token)
		assert.Error(t, err)
	})

	t.Run("invalid token", func(t *testing.T) {
		_, err := ts.ValidateJWTToken("invalid.token.value")
		assert.Error(t, err)
	})

	t.Run("wrong secret", func(t *testing.T) {
		req := &models.CreateTokenRequest{
			Type:      models.TokenTypeJWT,
			Sub:       "user3",
			Access:    models.AccessLevelRead,
			Scope:     []string{"scope1"},
			ExpiresAt: time.Now().Add(time.Minute),
		}
		token, err := ts.CreateJWTToken(req)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		wrongTS := NewTokenService(masterToken, "wrong-secret")
		_, err = wrongTS.ValidateJWTToken(token)
		assert.Error(t, err)
	})
}
