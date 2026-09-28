package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-123")
	require.NoError(t, err)
	assert.NotContains(t, hash, "correct-horse")
	assert.True(t, CheckPassword("correct-horse-123", hash))
	assert.False(t, CheckPassword("wrong", hash))
}

func TestPasswordValidation(t *testing.T) {
	_, err := HashPassword("short")
	assert.Error(t, err)
	_, err = HashPassword(strings.Repeat("a", 129))
	assert.Error(t, err)
}

func TestJWTRoundTrip(t *testing.T) {
	uid := uuid.New()
	raw, err := IssueToken("test-secret-32-chars-minimum-xyz", uid, time.Hour)
	require.NoError(t, err)

	got, err := ParseToken("test-secret-32-chars-minimum-xyz", raw)
	require.NoError(t, err)
	assert.Equal(t, uid, got)
}

func TestJWTRejectsBadSecretAndExpiry(t *testing.T) {
	uid := uuid.New()
	_, err := IssueToken("", uid, time.Hour)
	assert.Error(t, err)

	raw, err := IssueToken("s3cret-s3cret-s3cret-s3cret-1234", uid, -time.Hour)
	require.NoError(t, err)
	_, err = ParseToken("s3cret-s3cret-s3cret-s3cret-1234", raw)
	assert.Error(t, err, "expired token must be rejected")

	_, err = ParseToken("different-secret-different-secret", raw)
	assert.Error(t, err)
}

func TestAPIKeyFormatAndCheck(t *testing.T) {
	k, err := GenerateKey()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(k.PublicKey, PublicPrefix))
	assert.True(t, strings.HasPrefix(k.Secret, SecretPrefix))
	assert.True(t, CheckSecret(k.Secret, k.Hash))
	assert.False(t, CheckSecret("sk-lf-wrong", k.Hash))
}

func TestAPIKeyUniqueness(t *testing.T) {
	a, err := GenerateKey()
	require.NoError(t, err)
	b, err := GenerateKey()
	require.NoError(t, err)
	assert.NotEqual(t, a.PublicKey, b.PublicKey)
	assert.NotEqual(t, a.Secret, b.Secret)
}
