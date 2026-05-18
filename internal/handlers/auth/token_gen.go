package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const tokenBytesLen = 32

// generateOpaqueToken creates a new random opaque session token with the
// environment-appropriate prefix (st_live_ for production, st_test_ otherwise).
func generateOpaqueToken(isProduction bool) (plaintext, hash string, err error) {
	b := make([]byte, tokenBytesLen)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generateOpaqueToken: entropy read failed: %w", err)
	}
	raw := hex.EncodeToString(b)

	prefix := "st_test_"
	if isProduction {
		prefix = "st_live_"
	}
	plaintext = prefix + raw
	hash = hashToken(plaintext)
	return plaintext, hash, nil
}

// hashToken computes the SHA-256 hex digest of a token.
// This is what is stored in the database; the plaintext is never persisted.
func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
