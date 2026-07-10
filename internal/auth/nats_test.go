package auth

import (
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseXKeySeed(t *testing.T) {
	t.Run("empty seed returns nil key pair and no error", func(t *testing.T) {
		kp, err := parseXKeySeed("")
		require.NoError(t, err)
		assert.Nil(t, kp)
	})

	t.Run("valid seed returns a usable key pair", func(t *testing.T) {
		seedKp, err := nkeys.CreateCurveKeys()
		require.NoError(t, err)
		seed, err := seedKp.Seed()
		require.NoError(t, err)

		kp, err := parseXKeySeed(string(seed))
		require.NoError(t, err)
		require.NotNil(t, kp)

		wantPub, err := seedKp.PublicKey()
		require.NoError(t, err)
		gotPub, err := kp.PublicKey()
		require.NoError(t, err)
		assert.Equal(t, wantPub, gotPub)
	})

	t.Run("invalid seed returns an error", func(t *testing.T) {
		kp, err := parseXKeySeed("not-a-valid-seed")
		require.Error(t, err)
		assert.Nil(t, kp)
	})
}

func applyOptions(t *testing.T, opts []nats.Option) nats.Options {
	t.Helper()
	o := nats.Options{}
	for _, opt := range opts {
		require.NoError(t, opt(&o))
	}
	return o
}

func TestBuildNATSOptions(t *testing.T) {
	logger := slog.Default()

	t.Run("sets standard reconnect and handler options", func(t *testing.T) {
		opts := buildNATSOptions(logger, "", "")
		o := applyOptions(t, opts)

		assert.Equal(t, 5*time.Second, o.ReconnectWait)
		assert.Equal(t, -1, o.MaxReconnect)
		assert.NotNil(t, o.DisconnectedErrCB)
		assert.NotNil(t, o.ReconnectedCB)
		assert.NotNil(t, o.AsyncErrorCB)
		assert.Empty(t, o.User)
		assert.Empty(t, o.Password)
	})

	t.Run("adds user/password auth when both provided", func(t *testing.T) {
		opts := buildNATSOptions(logger, "alice", "secret")
		o := applyOptions(t, opts)

		assert.Equal(t, "alice", o.User)
		assert.Equal(t, "secret", o.Password)
	})

	t.Run("skips auth when only user or only password provided", func(t *testing.T) {
		o := applyOptions(t, buildNATSOptions(logger, "alice", ""))
		assert.Empty(t, o.User)
		assert.Empty(t, o.Password)

		o = applyOptions(t, buildNATSOptions(logger, "", "secret"))
		assert.Empty(t, o.User)
		assert.Empty(t, o.Password)
	})
}

func TestInitTokenCache_Disabled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("token_cache.enabled", false)

	c := &NATSClient{logger: slog.Default()}

	err := c.initTokenCache()
	require.NoError(t, err)
	assert.Nil(t, c.tokenCache)
}
