package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func inviteHash(token string) ([]byte, bool) {
	raw, ok := decodeFixed(token, 32)
	if !ok {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...), true
}

func deviceHash(raw []byte) []byte {
	sum := sha256.Sum256(raw)
	return append([]byte(nil), sum[:]...)
}

func decodeFixed(token string, n int) ([]byte, bool) {
	if raw, err := hex.DecodeString(token); err == nil && len(raw) == n && strings.EqualFold(hex.EncodeToString(raw), token) {
		return raw, true
	}
	if raw, err := base64.RawURLEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, true
	}
	if raw, err := base64.URLEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, true
	}
	if raw, err := base64.StdEncoding.DecodeString(token); err == nil && len(raw) == n {
		return raw, true
	}
	return nil, false
}
