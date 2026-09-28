package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword bcrypt-hashes a password for storage. Never store raw passwords.
func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return "", fmt.Errorf("password must be at most 128 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a raw password against a stored bcrypt hash.
func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
