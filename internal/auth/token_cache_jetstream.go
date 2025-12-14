package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
)

type JetStreamTokenCache struct {
	kv     nats.KeyValue
	secret []byte
	logger *slog.Logger
	bucket string
}

func NewJetStreamTokenCache(js nats.JetStreamContext, cfg TokenCacheConfig) (*JetStreamTokenCache, error) {
	logger := slog.With("component", "token_cache_jetstream")

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

	// Bind to the existing KV bucket or create it if missing.
	created := false
	kv, err := js.KeyValue(cfg.Bucket)
	if err != nil {
		if errors.Is(err, nats.ErrBucketNotFound) {
			kv, err = js.CreateKeyValue(&nats.KeyValueConfig{
				Bucket:   cfg.Bucket,
				TTL:      cfg.TTL,
				Replicas: cfg.Replicas,
			})
			if err == nil {
				created = true
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to access token cache bucket %q: %w", cfg.Bucket, err)
	}

	if created {
		logger.Info("Token cache bucket created (JetStream KV)",
			"bucket", cfg.Bucket,
			"ttl", cfg.TTL,
			"replicas", cfg.Replicas,
		)
	} else {
		logger.Info("Token cache bucket connected (JetStream KV)",
			"bucket", cfg.Bucket,
			"ttl", cfg.TTL,
			"replicas", cfg.Replicas,
		)
	}

	return &JetStreamTokenCache{kv: kv, secret: []byte(cfg.HMACSecret), logger: logger, bucket: cfg.Bucket}, nil
}

func (c *JetStreamTokenCache) Get(ctx context.Context, token string) (*TokenCacheEntry, error) {
	_ = ctx // nats.go KV API doesn't accept context in v1; keep for interface stability.

	key, err := tokenCacheKey(token, c.secret)
	if err != nil {
		return nil, err
	}
	keyPrefix := key
	if len(keyPrefix) > 12 {
		keyPrefix = keyPrefix[:12]
	}

	entry, err := c.kv.Get(key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			c.logger.Debug("Token cache miss",
				"bucket", c.bucket,
				"key_prefix", keyPrefix,
			)
			return nil, ErrTokenCacheMiss
		}
		c.logger.Warn("Token cache get failed",
			"bucket", c.bucket,
			"key_prefix", keyPrefix,
			"error", err,
		)
		return nil, err
	}

	out, err := unmarshalTokenCacheEntry(entry.Value())
	if err != nil {
		c.logger.Warn("Token cache entry unmarshal failed",
			"bucket", c.bucket,
			"key_prefix", keyPrefix,
			"revision", entry.Revision(),
			"error", err,
		)
		return nil, err
	}

	c.logger.Debug("Token cache hit",
		"bucket", c.bucket,
		"key_prefix", keyPrefix,
		"revision", entry.Revision(),
	)
	return out, nil
}

func (c *JetStreamTokenCache) Put(ctx context.Context, token string, entry TokenCacheEntry) error {
	_ = ctx // nats.go KV API doesn't accept context in v1; keep for interface stability.

	key, err := tokenCacheKey(token, c.secret)
	if err != nil {
		return err
	}
	keyPrefix := key
	if len(keyPrefix) > 12 {
		keyPrefix = keyPrefix[:12]
	}

	data, err := marshalTokenCacheEntry(entry)
	if err != nil {
		return err
	}

	rev, err := c.kv.Put(key, data)
	if err != nil {
		c.logger.Warn("Token cache put failed",
			"bucket", c.bucket,
			"key_prefix", keyPrefix,
			"error", err,
		)
		return err
	}
	// Never log plaintext tokens; only log the derived key prefix for correlation.
	c.logger.Debug("Token cache put ok",
		"bucket", c.bucket,
		"key_prefix", keyPrefix,
		"revision", rev,
	)
	return nil
}
