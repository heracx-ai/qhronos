package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/feedloop/qhronos/internal/models"
)

type TokenService struct {
	masterToken string
	jwtSecret   string
}

func NewTokenService(masterToken, jwtSecret string) *TokenService {
	return &TokenService{
		masterToken: masterToken,
		jwtSecret:   jwtSecret,
	}
}

// ValidateMasterToken checks if the provided token matches the master token
func (s *TokenService) ValidateMasterToken(token string) bool {
	return token == s.masterToken
}

// CreateJWTToken creates a new JWT token with the specified claims
func (s *TokenService) CreateJWTToken(req *models.CreateTokenRequest) (string, error) {
	// Create the claims
	claims := jwt.MapClaims{
		"sub":       req.Sub,
		"access":    req.Access,
		"scope":     req.Scope,
		"exp":       req.ExpiresAt.Unix(),
		"iat":       time.Now().Unix(),
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with the secret
	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateJWTToken validates a JWT token and returns its claims
func (s *TokenService) ValidateJWTToken(tokenString string) (*models.Token, error) {
	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	// Extract claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// Convert claims to Token model
	tokenModel := &models.Token{
		Sub:       claims["sub"].(string),
		Access:    models.AccessLevel(claims["access"].(string)),
		Scope:     convertInterfaceToStringSlice(claims["scope"]),
		ExpiresAt: time.Unix(int64(claims["exp"].(float64)), 0),
	}

	return tokenModel, nil
}

// ValidateScope checks if the token's scope includes all required tags
func (s *TokenService) ValidateScope(token *models.Token, requiredTags []string) bool {
	if token.Access == models.AccessLevelAdmin {
		return true
	}

	// Create a map of token's scope for efficient lookup
	scopeMap := make(map[string]bool)
	for _, tag := range token.Scope {
		scopeMap[tag] = true
	}

	// Check if all required tags are in the scope
	for _, tag := range requiredTags {
		if !scopeMap[tag] {
			return false
		}
	}

	return true
}

// ValidateAccess checks if the token has the required access level
func (s *TokenService) ValidateAccess(token *models.Token, requiredAccess models.AccessLevel) bool {
	// Define access level hierarchy
	accessLevels := map[models.AccessLevel]int{
		models.AccessLevelRead:      1,
		models.AccessLevelWrite:     2,
		models.AccessLevelReadWrite: 3,
		models.AccessLevelAdmin:     4,
	}

	// Admin has access to everything
	if token.Access == models.AccessLevelAdmin {
		return true
	}

	// Check if token's access level is sufficient
	return accessLevels[token.Access] >= accessLevels[requiredAccess]
}

// Helper function to convert interface{} to []string
func convertInterfaceToStringSlice(i interface{}) []string {
	slice := i.([]interface{})
	result := make([]string, len(slice))
	for i, v := range slice {
		result[i] = v.(string)
	}
	return result
} 