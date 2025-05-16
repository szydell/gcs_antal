package server

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"
)

func TestStart(t *testing.T) {
	tests := []struct {
		name           string
		host           string
		port           int
		timeout        time.Duration
		expectedHealth string
		expectMetrics  bool
	}{
		{
			name:           "Valid health and metrics endpoints",
			host:           "localhost",
			port:           8080,
			timeout:        5 * time.Second,
			expectedHealth: `{"status":true}`,
			expectMetrics:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(tt.host, tt.port, tt.timeout)
			errCh := make(chan error, 1)
			go func() {
				errCh <- s.Start()
			}()

			// Wait for server to start or timeout
			time.Sleep(100 * time.Millisecond)
			select {
			case err := <-errCh:
				if err != nil && err != http.ErrServerClosed {
					t.Fatalf("Failed to start server: %v", err)
				}
			default:
				// Server started successfully
			}
			defer func(s *Server, ctx context.Context) {
				err := s.Stop(ctx)
				if err != nil {
					t.Errorf("Failed to stop server: %v", err)
				}
			}(s, context.Background())

			// Health endpoint test
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/health", tt.host, tt.port))
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var healthResponse map[string]bool
			err = json.NewDecoder(resp.Body).Decode(&healthResponse)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedHealth, fmt.Sprintf(`{"status":%v}`, healthResponse["status"]))

			// Metrics endpoint test
			resp, err = http.Get(fmt.Sprintf("http://%s:%d/metrics", tt.host, tt.port))
			assert.NoError(t, err)
			assert.Equal(t, tt.expectMetrics, resp.StatusCode == http.StatusOK)
		})
	}
}

func TestStop(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		port        int
		timeout     time.Duration
		shouldError bool
	}{
		{
			name:        "Valid stop",
			host:        "localhost",
			port:        8080,
			timeout:     5 * time.Second,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(tt.host, tt.port, tt.timeout)

			go func() {
				_ = s.Start()
			}()

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout)
			defer cancel()

			err := s.Stop(ctx)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewServer(t *testing.T) {
	tests := []struct {
		name            string
		host            string
		port            int
		timeout         time.Duration
		expectedAddr    string
		expectedTimeout time.Duration
	}{
		{
			name:            "Valid host, port and timeout",
			host:            "localhost",
			port:            8080,
			timeout:         5 * time.Second,
			expectedAddr:    "localhost:8080",
			expectedTimeout: 5 * time.Second,
		},
		{
			name:            "Empty host",
			host:            "",
			port:            8080,
			timeout:         10 * time.Second,
			expectedAddr:    ":8080",
			expectedTimeout: 10 * time.Second,
		},
		{
			name:            "Zero port",
			host:            "127.0.0.1",
			port:            0,
			timeout:         2 * time.Second,
			expectedAddr:    "127.0.0.1:0",
			expectedTimeout: 2 * time.Second,
		},
		{
			name:            "Minimum timeout",
			host:            "example.com",
			port:            443,
			timeout:         1 * time.Nanosecond,
			expectedAddr:    "example.com:443",
			expectedTimeout: 1 * time.Nanosecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewServer(tt.host, tt.port, tt.timeout)

			assert.NotNil(t, s.server)
			assert.NotNil(t, s.logger)
			assert.Equal(t, tt.expectedAddr, s.server.Addr)
			assert.Equal(t, tt.expectedTimeout, s.server.ReadTimeout)
			assert.Equal(t, tt.expectedTimeout, s.server.WriteTimeout)
			assert.Equal(t, tt.expectedTimeout*2, s.server.IdleTimeout)
		})
	}
}
