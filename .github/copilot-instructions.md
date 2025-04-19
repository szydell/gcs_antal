
# NATS-GitLab Authentication Service

## Project Purpose

This microservice authenticates NATS clients using GitLab Personal Access Tokens. It serves as a bridge between NATS's `auth_callout` mechanism and GitLab's API.

## Core Architecture

- Written in Go (1.23+)
- Simple HTTP server for NATS `auth_callout` API
- GitLab API client for token verification
- YAML-based configuration

## Key Components

### `main.go`
- Entry point
- Loads configuration from YAML
- Initializes and runs HTTP server

### `config.yaml`
- Server settings (port, host, TLS)
- GitLab instance URL
- Timeouts and other operational parameters
- Logging configuration

### `internal/auth/handler.go`
- HTTP handler for NATS authentication requests
- Parses NATS `auth_callout` payloads
- Formats responses for NATS server
- Response format matches NATS expectations

### `internal/auth/gitlab.go`
- Contains GitLab authentication logic
- Takes GitLab username and PAT
- Verifies token against GitLab API
- Returns authentication decision
- Handles API error cases appropriately

### `internal/server/server.go`
- HTTP server setup
- Route configuration
- Middleware for logging, timeouts, etc.

## Authentication Flow

1. NATS client connects with GitLab username as NATS username and GitLab PAT as password
2. NATS server passes these credentials to our service via `auth_callout`
3. Our service calls GitLab API to verify the token
4. GitLab API response determines authentication success/failure
5. Our service responds to NATS with authorization decision
6. NATS server allows/denies client connection accordingly

## NATS `auth_callout` Contract

NATS sends a POST request with a JSON payload like:
```json
{
  "server_id": "NANMDBBUF7LZJNBLDNLP4T26BLQUGXH5WWVAKFXL3VYPH7FD2LPTSXYX",
  "client_id": 6,
  "subject": "connect",
  "host": "127.0.0.1:57224",
  "tags": null,
  "name": "sample-client",
  "lang": "go",
  "version": "1.11.0",
  "user": "gitlab_username",
  "password": "gitlab_pat_token"
}
```

Our service must respond with:
```json
{
  "ok": true,
  "permissions": {
    "publish": {
      "allow": ["topic.>"],
      "deny": ["private.>"]
    },
    "subscribe": {
      "allow": ["topic.>"],
      "deny": ["private.>"]
    }
  }
}
```

## Error Handling

- Invalid GitLab tokens should return `{"ok": false}` to NATS
- API timeouts should result in authentication failure
- GitLab API errors should be logged but not exposed in responses

## Code Style Guidelines

- Use standard Go project layout
- Follow Go best practices and idioms
- Use dependency injection for testability
- Comprehensive error handling
- Structured logging with appropriate levels
- Comments for exported functions

## Dependencies

Minimize dependencies, but the following are approved:
- `github.com/xanzy/go-gitlab` - Official GitLab API client
- `github.com/gin-gonic/gin` - HTTP routing (or standard library if preferred)
- `github.com/spf13/viper` - Configuration management
- Standard library packages

## Testing Strategy

- Unit tests for authentication logic
- Mock GitLab API for testing
- End-to-end tests for full request flow
- Test both success and failure cases

## Deployment / Environment

- Health check endpoint at `/health`
- Prometheus metrics at `/metrics`
- Sentry integration for error tracking (optional)
- Graceful shutdown


