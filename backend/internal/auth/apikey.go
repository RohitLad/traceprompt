package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// Key prefixes match Langfuse conventions so SDK docs carry over.
const (
	PublicPrefix = "pk-lf-"
	SecretPrefix = "sk-lf-"
)

// GeneratedKey is returned once at creation; only the hash is persisted.
type GeneratedKey struct {
	PublicKey string
	Secret    string
	Hash      string
}

// GenerateKey creates a pk-lf-/sk-lf- pair and its SHA-256 hash.
// Secrets are 32 random bytes (256-bit), hex-encoded for copy-paste safety.
func GenerateKey() (GeneratedKey, error) {
	pub, err := randomHex(16)
	if err != nil {
		return GeneratedKey{}, err
	}
	sec, err := randomHex(32)
	if err != nil {
		return GeneratedKey{}, err
	}
	secret := SecretPrefix + sec
	return GeneratedKey{
		PublicKey: PublicPrefix + pub,
		Secret:    secret,
		Hash:      HashSecret(secret),
	}, nil
}

// HashSecret returns the hex SHA-256 of a raw secret for storage.
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// CheckSecret compares a candidate secret against a stored hash
// in constant time to avoid timing side-channels.
func CheckSecret(candidate, hash string) bool {
	candidateHash := HashSecret(candidate)
	if len(candidateHash) != len(hash) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidateHash), []byte(hash)) == 1
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
