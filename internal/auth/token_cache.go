package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

var (
	ErrTokenCacheMiss = errors.New("token cache miss")
	ErrInvalidToken   = errors.New("invalid token")
)

// TokenCacheEntry is the value stored in JetStream KV.
//
// NOTE: Never store plaintext tokens.
type TokenCacheEntry struct {
	Username       string `json:"username"`
	Scopes         string `json:"scopes"`
	LastVerifiedAt string `json:"last_verified_at"`
}

// TokenCache is a token cache implemented ONLY via NATS JetStream Key-Value.
//
// Implementations must:
//   - Use HMAC-SHA256(token, secret) as the KV key
//   - never persist or log plaintext tokens
//   - rely on KV MaxAge for TTL enforcement
type TokenCache interface {
	Get(ctx context.Context, token string) (*TokenCacheEntry, error)
	Put(ctx context.Context, token string, entry TokenCacheEntry) error
}

func tokenCacheKey(token string, secret []byte) (string, error) {
	if token == "" {
		return "", ErrInvalidToken
	}
	if len(secret) == 0 {
		return "", errors.New("token cache hmac_secret is empty")
	}

	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(token))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

func marshalTokenCacheEntry(entry TokenCacheEntry) ([]byte, error) {
	return json.Marshal(entry)
}

func unmarshalTokenCacheEntry(b []byte) (*TokenCacheEntry, error) {
	var out TokenCacheEntry
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
