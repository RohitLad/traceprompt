package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenTTL is the UI session lifetime. Short enough to limit replay,
// long enough to avoid annoying re-logins.
const TokenTTL = 24 * time.Hour

// Claims carried in UI JWTs.
type Claims struct {
	UserID uuid.UUID `json:"uid"`
	jwt.RegisteredClaims
}

// IssueToken mints a signed JWT for a user.
func IssueToken(secret string, userID uuid.UUID, ttl time.Duration) (string, error) {
	//nolint:gosec // compares against the documented dev placeholder, not a real secret
	if secret == "" || secret == "dev-only-change-me" {
		return "", fmt.Errorf("jwt secret not configured")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return tok.SignedString([]byte(secret))
}

// ParseToken validates a JWT and returns the embedded user ID.
func ParseToken(secret, raw string) (uuid.UUID, error) {
	tok, err := jwt.ParseWithClaims(raw, &Claims{}, func(_ *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return uuid.Nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return uuid.Nil, fmt.Errorf("invalid token")
	}
	return claims.UserID, nil
}
