package auth

import (
	"time"

	"github.com/spf13/viper"
)

type TokenCacheConfig struct {
	Enabled    bool
	TTL        time.Duration
	Bucket     string
	Replicas   int
	HMACSecret string
}

func LoadTokenCacheConfig() TokenCacheConfig {
	return TokenCacheConfig{
		Enabled:    viper.GetBool("token_cache.enabled"),
		TTL:        viper.GetDuration("token_cache.ttl"),
		Bucket:     viper.GetString("token_cache.bucket"),
		Replicas:   viper.GetInt("token_cache.replicas"),
		HMACSecret: viper.GetString("token_cache.hmac_secret"),
	}
}
