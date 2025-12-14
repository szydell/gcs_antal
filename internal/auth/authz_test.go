package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockGitLabVerifier struct {
	verify func(token string) (*VerifiedToken, error)
}

func (m mockGitLabVerifier) VerifyTokenInfo(token string) (*VerifiedToken, error) {
	return m.verify(token)
}

type mockSharedKV struct {
	now func() time.Time
	ttl time.Duration

	data map[string]mockKVRecord
}

type mockKVRecord struct {
	value    TokenCacheEntry
	storedAt time.Time
}

type mockTokenCache struct {
	secret []byte
	kv     *mockSharedKV

	getCalls int
	putCalls int
}

func (m *mockTokenCache) ResetCounts() {
	m.getCalls = 0
	m.putCalls = 0
}

func (m *mockTokenCache) GetCalls() int { return m.getCalls }
func (m *mockTokenCache) PutCalls() int { return m.putCalls }

func (m *mockTokenCache) Get(ctx context.Context, token string) (*TokenCacheEntry, error) {
	_ = ctx
	m.getCalls++
	key, err := tokenCacheKey(token, m.secret)
	if err != nil {
		return nil, err
	}
	rec, ok := m.kv.data[key]
	if !ok {
		return nil, ErrTokenCacheMiss
	}
	if m.kv.ttl > 0 && m.kv.now().Sub(rec.storedAt) > m.kv.ttl {
		delete(m.kv.data, key)
		return nil, ErrTokenCacheMiss
	}
	out := rec.value
	return &out, nil
}

func (m *mockTokenCache) Put(ctx context.Context, token string, entry TokenCacheEntry) error {
	_ = ctx
	m.putCalls++
	key, err := tokenCacheKey(token, m.secret)
	if err != nil {
		return err
	}
	if m.kv.data == nil {
		m.kv.data = make(map[string]mockKVRecord)
	}
	m.kv.data[key] = mockKVRecord{value: entry, storedAt: m.kv.now()}
	return nil
}

func TestAuthorizeToken_HappyPath_WritesCache(t *testing.T) {
	ctx := context.Background()

	clock := time.Date(2025, 12, 14, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }

	kv := &mockSharedKV{now: now, ttl: 24 * time.Hour, data: map[string]mockKVRecord{}}
	cache := &mockTokenCache{secret: []byte("secret"), kv: kv}

	verifier := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		require.Equal(t, "glpat-valid", token)
		return &VerifiedToken{Username: "tester", Scopes: []string{"read_api", "read_user"}}, nil
	}}

	res, err := AuthorizeToken(ctx, "glpat-valid", verifier, cache, now)
	require.NoError(t, err)
	require.True(t, res.Allow)
	require.False(t, res.FromCache)
	require.NotNil(t, res.Verified)
	require.Nil(t, res.CacheWriteErr)
	require.Equal(t, 0, cache.GetCalls())
	require.Equal(t, 1, cache.PutCalls())
}

func TestAuthorizeToken_CacheFallback_OnTimeout_AllowsOnHit(t *testing.T) {
	ctx := context.Background()

	clock := time.Date(2025, 12, 14, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }

	kv := &mockSharedKV{now: now, ttl: 24 * time.Hour, data: map[string]mockKVRecord{}}
	cache := &mockTokenCache{secret: []byte("secret"), kv: kv}

	// Pre-populate cache.
	require.NoError(t, cache.Put(ctx, "glpat-cached", TokenCacheEntry{Username: "tester", Scopes: "read_api", LastVerifiedAt: now().Format(time.RFC3339)}))
	cache.ResetCounts()

	verifier := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		return nil, context.DeadlineExceeded
	}}

	res, err := AuthorizeToken(ctx, "glpat-cached", verifier, cache, now)
	require.NoError(t, err)
	require.True(t, res.Allow)
	require.True(t, res.FromCache)
	require.Nil(t, res.Verified)
	require.Equal(t, 1, cache.GetCalls())
	require.Equal(t, 0, cache.PutCalls())
}

func TestAuthorizeToken_InvalidToken_DoesNotCheckCache(t *testing.T) {
	ctx := context.Background()

	now := func() time.Time { return time.Date(2025, 12, 14, 12, 0, 0, 0, time.UTC) }
	kv := &mockSharedKV{now: now, ttl: 24 * time.Hour, data: map[string]mockKVRecord{}}
	cache := &mockTokenCache{secret: []byte("secret"), kv: kv}

	// Pre-populate cache, then reset counters to ensure we only observe calls from authorization.
	require.NoError(t, cache.Put(ctx, "glpat-invalid", TokenCacheEntry{Username: "tester", Scopes: "read_api", LastVerifiedAt: now().Format(time.RFC3339)}))
	cache.ResetCounts()

	verifier := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		return nil, ErrInvalidToken
	}}

	res, err := AuthorizeToken(ctx, "glpat-invalid", verifier, cache, now)
	require.NoError(t, err)
	require.False(t, res.Allow)
	require.False(t, res.FromCache)
	require.Equal(t, 0, cache.GetCalls())
	require.Equal(t, 0, cache.PutCalls())
}

func TestAuthorizeToken_CacheExpired_DeniesOnMiss(t *testing.T) {
	ctx := context.Background()

	clock := time.Date(2025, 12, 14, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	kv := &mockSharedKV{now: now, ttl: 1 * time.Second, data: map[string]mockKVRecord{}}
	cache := &mockTokenCache{secret: []byte("secret"), kv: kv}

	// Write an entry, then advance time beyond TTL.
	require.NoError(t, cache.Put(ctx, "glpat-expiring", TokenCacheEntry{Username: "tester", Scopes: "read_api", LastVerifiedAt: now().Format(time.RFC3339)}))
	cache.ResetCounts()
	clock = clock.Add(2 * time.Second)

	verifier := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		return nil, context.DeadlineExceeded
	}}

	res, err := AuthorizeToken(ctx, "glpat-expiring", verifier, cache, now)
	require.NoError(t, err)
	require.False(t, res.Allow)
	require.Equal(t, 1, cache.GetCalls())
}

func TestAuthorizeToken_MultiInstance_SharedKV(t *testing.T) {
	ctx := context.Background()

	clock := time.Date(2025, 12, 14, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }

	shared := &mockSharedKV{now: now, ttl: 24 * time.Hour, data: map[string]mockKVRecord{}}
	cacheA := &mockTokenCache{secret: []byte("secret"), kv: shared}
	cacheB := &mockTokenCache{secret: []byte("secret"), kv: shared}

	verifierOK := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		return &VerifiedToken{Username: "tester", Scopes: []string{"read_api"}}, nil
	}}
	verifierTimeout := mockGitLabVerifier{verify: func(token string) (*VerifiedToken, error) {
		return nil, context.DeadlineExceeded
	}}

	resA, err := AuthorizeToken(ctx, "glpat-shared", verifierOK, cacheA, now)
	require.NoError(t, err)
	require.True(t, resA.Allow)
	require.Equal(t, 1, cacheA.PutCalls())

	cacheB.ResetCounts()
	resB, err := AuthorizeToken(ctx, "glpat-shared", verifierTimeout, cacheB, now)
	require.NoError(t, err)
	require.True(t, resB.Allow)
	require.True(t, resB.FromCache)
	require.Equal(t, 1, cacheB.GetCalls())
	require.Equal(t, 0, cacheB.PutCalls())
}
