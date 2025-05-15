package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
