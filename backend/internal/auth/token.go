package auth

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"ocm-backend/internal/logging"
)

const tokenTTL = 24 * time.Hour

// TokenService issues and validates JWT access tokens.
type TokenService struct {
	secret []byte
}

// NewTokenService reads JWT_SECRET from the environment. A fixed development
// fallback keeps local setup frictionless; set JWT_SECRET in production.
func NewTokenService() *TokenService {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "ocm-dev-secret-do-not-use-in-production"
		logging.L.Warn("auth: JWT_SECRET not set, using built-in development secret -- set JWT_SECRET in production")
	}
	return &TokenService{secret: []byte(secret)}
}

// Issue creates a signed token for an authenticated user.
func (s *TokenService) Issue(u User) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   u.Username,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(tokenTTL)),
		Issuer:    "ocm-backend",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Parse validates a token and returns the username it was issued to.
func (s *TokenService) Parse(tokenString string) (string, error) {
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", errors.New("invalid or expired token")
	}
	return claims.Subject, nil
}
