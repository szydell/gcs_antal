package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
)

type JetStreamTokenCache struct {
	kv     nats.KeyValue
	secret []byte
}

func NewJetStreamTokenCache(js nats.JetStreamContext, cfg TokenCacheConfig) (*JetStreamTokenCache, error) {
	if js == nil {
		return nil, errors.New("jetstream context is nil")
	}
	if cfg.Bucket == "" {
		return nil, errors.New("token_cache.bucket is empty")
	}
	if cfg.TTL <= 0 {
		return nil, errors.New("token_cache.ttl must be > 0")
	}
	if cfg.Replicas <= 0 {
		cfg.Replicas = 3
	}
	if cfg.HMACSecret == "" {
		return nil, errors.New("token_cache.hmac_secret is required when token_cache.enabled is true")
	}

	// Bind to existing KV bucket, or create it if missing.
	kv, err := js.KeyValue(cfg.Bucket)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
				Bucket:   cfg.Bucket,
				TTL:      cfg.TTL,
				Replicas: cfg.Replicas,
			})
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to access token cache bucket %q: %w", cfg.Bucket, err)
	}

	return &JetStreamTokenCache{kv: kv, secret: []byte(cfg.HMACSecret)}, nil
}

func (c *JetStreamTokenCache) Get(ctx context.Context, token string) (*TokenCacheEntry, error) {
	_ = ctx // nats.go KV API doesn't accept context in v1; keep for interface stability.

	key, err := tokenCacheKey(token, c.secret)
	if err != nil {
		return nil, err
	}

	entry, err := c.kv.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil, ErrTokenCacheMiss
		}
		return nil, err
	}

	return unmarshalTokenCacheEntry(entry.Value())
}

func (c *JetStreamTokenCache) Put(ctx context.Context, token string, entry TokenCacheEntry) error {
	_ = ctx // nats.go KV API doesn't accept context in v1; keep for interface stability.

	key, err := tokenCacheKey(token, c.secret)
	if err != nil {
		return err
	}

	data, err := marshalTokenCacheEntry(entry)
	if err != nil {
		return err
	}

	_, err = c.kv.Put(key, data)
	return err
}
