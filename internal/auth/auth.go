// Package auth provides one-time token helpers for supervised plugin bootstrap.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// NewToken returns a fresh random bootstrap token.
func NewToken() ([]byte, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	return buf, nil
}

// EqualToken compares two tokens in constant time.
func EqualToken(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

// Encode base64-encodes raw bytes.
func Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Decode base64-decodes raw bytes.
func Decode(data string) ([]byte, error) {
	out, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode base64: %w", err)
	}
	return out, nil
}
