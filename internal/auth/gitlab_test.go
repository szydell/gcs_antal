package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// Mock GitLab client for better test isolation
type mockGitLabClient struct {
	client *GitLabClient
	// Create a separate HTTP client to avoid the real one making extra requests
	httpClient *http.Client
}

func newMockGitLabClient(server *httptest.Server) *mockGitLabClient {
	client := &mockGitLabClient{
		client: &GitLabClient{
			baseURL:           server.URL,
			timeout:           1 * time.Second,
			retries:           2, // 3 attempts total (initial + 2 retries)
			retryDelaySeconds: 0, // No delay for faster tests
		},
		httpClient: server.Client(),
	}
	return client
}

// VerifyToken delegates to the underlying GitLabClient
func (m *mockGitLabClient) VerifyToken(token string) (bool, error) {
	return m.client.VerifyToken(token)
}

func TestVerifyToken(t *testing.T) {
	// Reset viper config before each test
	viper.Reset()

	t.Run("successful token verification", func(t *testing.T) {
		// Setup test server
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"id": 1, "username": "tester"}`))
			assert.NoError(t, err)
		}))
		defer testServer.Close()

		// Configure mock client
		client := newMockGitLabClient(testServer)

		// Test
		valid, err := client.VerifyToken("valid_token")

		// Assertions
		assert.True(t, valid)
		assert.NoError(t, err)
	})

	t.Run("unauthorized token", func(t *testing.T) {
		// Setup test server
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, err := w.Write([]byte(`{"message": "401 Unauthorized"}`))
			assert.NoError(t, err)
		}))
		defer testServer.Close()

		// Configure mock client
		client := newMockGitLabClient(testServer)

		// Test
		valid, err := client.VerifyToken("invalid_token")

		// Assertions
		assert.False(t, valid)
		assert.NoError(t, err)
	})

	t.Run("retry on transient error then succeed", func(t *testing.T) {
		// Setup counter to track the number of requests
		requestCount := 0

		// Setup test server
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Fail the first two attempts with 500 error
			if requestCount < 2 {
				requestCount++
				w.WriteHeader(http.StatusInternalServerError)
				_, err := w.Write([]byte(`{"message": "Internal Server Error"}`))
				assert.NoError(t, err)
				return
			}

			// Succeed on the third attempt
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"id": 1, "username": "tester"}`))
			assert.NoError(t, err)
			requestCount++ // Still increment the counter on success
		}))
		defer testServer.Close()

		// Configure mock client
		client := newMockGitLabClient(testServer)

		// Test
		valid, err := client.VerifyToken("valid_token")

		// Assertions
		assert.True(t, valid)
		assert.NoError(t, err)
		assert.Equal(t, 3, requestCount) // Should include the successful request
	})

	t.Run("all retries exhausted", func(t *testing.T) {
		// Mock the time.Sleep function to avoid delays
		originalSleep := timeSleep
		timeSleep = func(d time.Duration) {} // No-op sleep for faster tests
		defer func() { timeSleep = originalSleep }()

		// Setup counter to track the number of requests
		requestCount := 0

		// Setup test server
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only count requests to the actual user endpoint
			if r.URL.Path == "/api/v4/user" && r.Method == "GET" {
				requestCount++
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte(`{"message": "Internal Server Error"}`))
			assert.NoError(t, err)
		}))
		defer testServer.Close()

		// Create a mock client
		client := newMockGitLabClient(testServer)

		// Test
		valid, err := client.VerifyToken("valid_token")

		// Assertions
		assert.False(t, valid)
		assert.Error(t, err)
		assert.Equal(t, 6, requestCount) // Expect 6 requests (2 per each of 3 attempts)
	})

	t.Run("nil user response", func(t *testing.T) {
		// Setup test server that returns a nil user
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{}`))
			assert.NoError(t, err)
		}))
		defer testServer.Close()

		client := newMockGitLabClient(testServer)
		valid, err := client.VerifyToken("any_token")

		assert.False(t, valid)
		assert.NoError(t, err)
	})

	t.Run("retry delay is respected", func(t *testing.T) {
		// Mock time.Sleep to track delays
		originalSleep := timeSleep
		sleepCalled := 0
		sleepDurations := make([]time.Duration, 0)

		// Replace time.Sleep with the mock version
		timeSleep = func(d time.Duration) {
			sleepCalled++
			sleepDurations = append(sleepDurations, d)
		}
		defer func() { timeSleep = originalSleep }()

		// Setup test server that always fails
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer testServer.Close()

		// Configure mock client with custom delay
		mock := newMockGitLabClient(testServer)
		mock.client.retryDelaySeconds = 3 * time.Second // Override delay for this specific test

		// Test
		valid, err := mock.VerifyToken("token")

		// Assertions
		assert.False(t, valid)
		assert.Error(t, err)
		assert.Equal(t, 2, sleepCalled) // Should sleep twice between 3 attempts
		for _, duration := range sleepDurations {
			assert.Equal(t, 3*time.Second, duration) // Each delay should be 3 seconds
		}
	})
}
