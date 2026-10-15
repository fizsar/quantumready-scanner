package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewToken returns a random 256-bit token (URL-safe text) and its SHA-256.
// Only the hash is stored: a database leak does not give working sessions or
// links.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is the stored form of a token.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
