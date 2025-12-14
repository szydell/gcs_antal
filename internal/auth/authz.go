package auth

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

type GitLabVerifier interface {
	VerifyTokenInfo(token string) (*VerifiedToken, error)
}

type AuthorizeResult struct {
	Allow     bool
	FromCache bool
	// Verified is populated when GitLab verification succeeded.
	Verified *VerifiedToken
	// CacheWriteErr is set when GitLab verification succeeds, but writing to KV fails.
	// Authorization should still proceed (ALLOW) in that case.
	CacheWriteErr error
}

// AuthorizeToken implements the strict authorization flow:
//  1. Always call GitLab first.
//  2. If GitLab returns invalid token (401): deny immediately, do not check cache.
//  3. If GitLab returns timeout/network/5xx: fallback to token cache (JetStream KV).
//  4. Cache hit (and not expired via KV TTL): allow.
func AuthorizeToken(ctx context.Context, token string, verifier GitLabVerifier, cache TokenCache, now func() time.Time) (AuthorizeResult, error) {
	vt, err := verifier.VerifyTokenInfo(token)
	if err == nil {
		res := AuthorizeResult{Allow: true, Verified: vt}
		if cache != nil {
			err := cache.Put(ctx, token, TokenCacheEntry{
				Username:       vt.Username,
				Scopes:         strings.Join(vt.Scopes, ","),
				LastVerifiedAt: now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				res.CacheWriteErr = err
			}
		}
		return res, nil
	}
	if errors.Is(err, ErrInvalidToken) {
		return AuthorizeResult{Allow: false}, nil
	}

	if cache != nil && isFallbackToCacheError(err) {
		_, cErr := cache.Get(ctx, token)
		if cErr == nil {
			return AuthorizeResult{Allow: true, FromCache: true}, nil
		}
		if errors.Is(cErr, ErrTokenCacheMiss) {
			return AuthorizeResult{Allow: false}, nil
		}
		return AuthorizeResult{Allow: false}, cErr
	}

	return AuthorizeResult{Allow: false}, err
}

func statusCodeFromGitLabError(err error) (int, bool) {
	var errResp *gitlab.ErrorResponse
	if errors.As(err, &errResp) && errResp != nil && errResp.Response != nil {
		return errResp.Response.StatusCode, true
	}
	return 0, false
}

func isFallbackToCacheError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() || nerr.Temporary() {
			return true
		}
	}

	var uerr *url.Error
	if errors.As(err, &uerr) {
		if uerr.Timeout() {
			return true
		}
		var nerr2 net.Error
		if errors.As(uerr, &nerr2) {
			if nerr2.Timeout() || nerr2.Temporary() {
				return true
			}
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}

	if code, ok := statusCodeFromGitLabError(err); ok {
		return code >= 500
	}

	return false
}
