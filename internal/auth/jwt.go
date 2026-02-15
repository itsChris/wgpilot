package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT payload for wgpilot sessions.
type Claims struct {
	jwt.RegisteredClaims
	Username string `json:"username"`
	Role     string `json:"role"`
}

// JWTService handles JWT generation and validation.
type JWTService struct {
	secret []byte
	ttl    time.Duration
	logger *slog.Logger
}

// NewJWTService creates a JWT service with the given signing secret and token TTL.
func NewJWTService(secret []byte, ttl time.Duration, logger *slog.Logger) (*JWTService, error) {
	if len(secret) < 32 {
		return nil, fmt.Errorf("jwt: secret must be at least 32 bytes, got %d", len(secret))
	}
	return &JWTService{
		secret: secret,
		ttl:    ttl,
		logger: logger,
	}, nil
}

// TTL returns the configured token lifetime.
func (s *JWTService) TTL() time.Duration {
	return s.ttl
}

// Generate creates a signed JWT for the given user.
func (s *JWTService) Generate(userID int64, username, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
		Username: username,
		Role:     role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("jwt: sign token: %w", err)
	}
	return signed, nil
}

// Validate parses and validates a JWT string, returning the claims if valid.
func (s *JWTService) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("jwt: parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwt: invalid token claims")
	}

	return claims, nil
}

// GenerateSecret creates a cryptographically random secret of the given byte length.
func GenerateSecret(length int) ([]byte, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	return b, nil
}

type userContextKey struct{}

// WithUser stores JWT claims in the context.
func WithUser(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, userContextKey{}, claims)
}

// UserFromContext extracts JWT claims from the context.
// Returns nil if no user is set.
func UserFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(userContextKey{}).(*Claims)
	return claims
}
