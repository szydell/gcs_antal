package auth

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestLoadTokenCacheConfig_ReadsValuesFromViper(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("token_cache.enabled", true)
	viper.Set("token_cache.ttl", "6h")
	viper.Set("token_cache.bucket", "bucket_a")
	viper.Set("token_cache.replicas", 2)
	viper.Set("token_cache.hmac_secret", "secret")

	cfg := LoadTokenCacheConfig()
	require.True(t, cfg.Enabled)
	require.Equal(t, 6*time.Hour, cfg.TTL)
	require.Equal(t, "bucket_a", cfg.Bucket)
	require.Equal(t, 2, cfg.Replicas)
	require.Equal(t, "secret", cfg.HMACSecret)
}
