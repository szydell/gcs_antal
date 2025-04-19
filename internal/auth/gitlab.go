package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/spf13/viper"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabClient handles interactions with GitLab API
type GitLabClient struct {
	baseURL string
	timeout time.Duration
}

// NewGitLabClient creates a new GitLab client
func NewGitLabClient() *GitLabClient {
	return &GitLabClient{
		baseURL: viper.GetString("gitlab.url"),
		timeout: time.Duration(viper.GetInt("gitlab.timeout")) * time.Second,
	}
}

// VerifyToken checks if the provided token is valid for the given username
func (c *GitLabClient) VerifyToken(username, token string) (bool, error) {
	logger := slog.With("service", "gitlab", "username", username)
	logger.Debug("Verifying GitLab token")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Initialize GitLab client with the user's token and custom base URL
	git, err := gitlab.NewClient(token, gitlab.WithBaseURL(fmt.Sprintf("%s/api/v4", c.baseURL)))
	if err != nil {
		logger.Error("Failed to create GitLab client", "error", err)
		sentry.CaptureException(err)
		return false, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	// Try to get the current user (token owner)
	user, _, err := git.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		// Check if it's an authentication error (401 Unauthorized)
		if isUnauthorizedError(err) {
			logger.Info("GitLab token validation failed", "error", err)
			return false, nil
		}

		// Other errors (network, timeout, etc.)
		logger.Error("Error calling GitLab API", "error", err)
		sentry.CaptureException(err)
		return false, fmt.Errorf("error calling GitLab API: %w", err)
	}

	// No user returned
	if user == nil {
		logger.Warn("GitLab API returned nil user")
		return false, nil
	}

	// Check if username matches
	if user.Username != username {
		logger.Info("Username mismatch",
			"provided", username,
			"actual", user.Username)
		return false, nil
	}

	logger.Info("GitLab token verification successful")
	return true, nil
}

// isUnauthorizedError checks if the error is an HTTP 401 Unauthorized error
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}

	// Type assertion to check for *gitlab.ErrorResponse
	var errResp *gitlab.ErrorResponse
	if errors.As(err, &errResp) {
		return errResp.Response.StatusCode == 401
	}

	return false
}
