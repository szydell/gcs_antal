# NATS-GitLab Authentication Service

## Project Purpose

This microservice authenticates NATS clients using GitLab Personal Access Tokens. It serves as a bridge between NATS's `auth_callout` mechanism and GitLab's API.

## Core Architecture

- Written in Go (1.23+)
- Simple NATS client for NATS `auth_callout` API
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

### `internal/auth/gitlab.go`
- Contains GitLab authentication logic
- Takes GitLab PAT
- Verifies token against GitLab API
- Returns authentication decision
- Handles API error cases appropriately

### `internal/server/server.go`
- HTTP server setup
- Route configuration
- Middleware for logging, timeouts, etc.

### `internal/auth/nats.go`
- NATS client to act as a auth_callout
- Connects to NATS server
- Handles authentication requests
- Sends responses back to NATS server

## Authentication Flow

1. NATS client connects with some username as NATS username and GitLab PAT as password
2. NATS server passes these credentials to our service via `auth_callout`
3. Our service extracts password (which is a PAT) from the NATS request
4. Our service calls GitLab API to verify the token
5. GitLab API response determines authentication success/failure
6. Our service responds to NATS with authorization decision
7. NATS server allows/denies client connection accordingly

## NATS `auth_callout` Contract

NATS sends a request with the following structure:
```json
{
  "nats": {
    "server_id": "NANMDBBUF7LZJNBLDNLP4T26BLQUGXH5WWVAKFXL3VYPH7FD2LPTSXYX",
    "client_info": {
      "host": "127.0.0.1",
      "port": 57224,
      "id": 6,
      "user": "gitlab_username",
      "name": "sample-client",
      "tags": null,
      "lang": "go",
      "version": "1.11.0",
      "protocol": 1,
      "account": "",
      "jwt": "",
      "issuer_key": "",
      "name_tag": "",
      "kind": 0,
      "client_type": 2,
      "client_ip": "127.0.0.1"
    },
    "connect_opts": {
      "host": "127.0.0.1",
      "port": 4222,
      "username": "gitlab_username",
      "password": "gitlab_pat_token",
      "name": "sample-client",
      "lang": "go",
      "version": "1.11.0"
    },
    "client_tls": null
  }
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
- `gitlab.com/gitlab-org/api/client-go` - Official GitLab API client
- HTTP routing - standard library
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


