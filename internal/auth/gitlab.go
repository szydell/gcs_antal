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
	baseURL           string
	timeout           time.Duration
	retries           int
	retryDelaySeconds time.Duration
}

// NewGitLabClient creates a new GitLab client
func NewGitLabClient() *GitLabClient {
	return &GitLabClient{
		baseURL:           viper.GetString("gitlab.url"),
		timeout:           time.Duration(viper.GetInt("gitlab.timeout")) * time.Second,
		retries:           viper.GetInt("gitlab.retries"),
		retryDelaySeconds: time.Duration(viper.GetInt("gitlab.retryDelaySeconds")) * time.Second,
	}
}

// VerifyToken checks if the provided token is valid
func (c *GitLabClient) VerifyToken(token string) (bool, error) {
	logger := slog.With("service", "gitlab")
	logger.Debug("Verifying GitLab token")

	// Initialize the GitLab client with the user's token and custom base URL
	git, err := gitlab.NewClient(token, gitlab.WithBaseURL(fmt.Sprintf("%s/api/v4", c.baseURL)))
	if err != nil {
		logger.Error("Failed to create GitLab client", "error", err)
		sentry.CaptureException(err)
		return false, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	// Try to get the current user (token owner) with retries
	maxAttempts := c.retries + 1
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Create fresh context with timeout for each attempt
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		user, _, err := git.Users.CurrentUser(gitlab.WithContext(ctx))
		cancel() // Cancel immediately after the call

		if err == nil {
			if user == nil {
				logger.Warn("GitLab API returned nil user")
				return false, nil
			}
			logger.Info("GitLab token verification successful", "token_username", user.Username)
			return true, nil
		}

		// Check if it's an authentication error (401 Unauthorized)
		if isUnauthorizedError(err) {
			logger.Info("GitLab token validation failed", "error", err)
			return false, nil
		}

		// Store the error for potential retry
		lastErr = err

		// Check if we should retry
		if attempt < maxAttempts-1 {
			delay := c.retryDelaySeconds
			logger.Warn("GitLab API call failed, retrying", "attempt", attempt+1, "max_attempts", maxAttempts, "error", err)
			time.Sleep(delay)
		}
	}

	// All attempts failed
	logger.Error("Error calling GitLab API after all retries", "error", lastErr)
	sentry.CaptureException(lastErr)
	return false, fmt.Errorf("error calling GitLab API after %d attempts: %w", maxAttempts, lastErr)
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
