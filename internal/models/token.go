package models

import "time"

// TokenType represents the type of token
type TokenType string

const (
	TokenTypeJWT TokenType = "jwt"
)

// AccessLevel represents the access level of a token
type AccessLevel string

const (
	AccessLevelRead      AccessLevel = "read"
	AccessLevelWrite     AccessLevel = "write"
	AccessLevelReadWrite AccessLevel = "read_write"
	AccessLevelAdmin     AccessLevel = "admin"
)

// Token represents a JWT token with its claims
type Token struct {
	Sub       string      `json:"sub"`
	Access    AccessLevel `json:"access"`
	Scope     []string    `json:"scope"`
	ExpiresAt time.Time   `json:"exp"`
}

// TokenClaims represents the claims in a JWT token
type TokenClaims struct {
	Sub       string       `json:"sub"`
	Access    AccessLevel  `json:"access"`
	Scope     []string    `json:"scope"`
	ExpiresAt time.Time   `json:"exp"`
}

// CreateTokenRequest represents the request to create a new token
type CreateTokenRequest struct {
	Type      TokenType   `json:"type" binding:"required"`
	Sub       string      `json:"sub" binding:"required"`
	Access    AccessLevel `json:"access" binding:"required"`
	Scope     []string    `json:"scope" binding:"required"`
	ExpiresAt time.Time   `json:"expires_at" binding:"required"`
}

// CreateTokenResponse represents the response when creating a new token
type CreateTokenResponse struct {
	Token     string      `json:"token"`
	Type      TokenType   `json:"type"`
	Sub       string      `json:"sub"`
	Access    AccessLevel `json:"access"`
	Scope     []string    `json:"scope"`
	ExpiresAt time.Time   `json:"expires_at"`
} 