# GCS Antal
## NATS-GitLab Authentication Service

A lightweight authentication microservice that bridges NATS `auth_callout` authentication with GitLab Personal Access Tokens.

## Overview

GCS Antal enables seamless authentication of NATS clients using GitLab credentials. It acts as a middleware between NATS servers and GitLab's API, verifying tokens and providing appropriate permissions to authenticated clients.

### Key Features

- **Simplified Authentication**: Use GitLab PATs for NATS authentication
- **Flexible Permissions**: Configure publish/subscribe permissions in YAML
- **Monitoring Ready**: Built-in health checks and Prometheus metrics
- **Easy Configuration**: Simple YAML-based setup

## Setup

### Prerequisites

- Go 1.23+
- GitLab instance with API access
- NATS server (v2.10.0+) with `auth_callout` support
- NATS `nk` tool for key generation

### Configuration

#### 1. Generate NATS NKeys

```bash
# Generate an account key pair for signing
nk gen account -pub SYS.pub -priv SYS.nk
# or
nsc generate nkey --account
```

- The **private key** (starting with `SA`) goes in your `config.yaml` as `nats.issuer_seed`
- The **public key** (starting with `A`) is used in your NATS server config as `issuer`

#### 2. Configure GCS Antal

Copy and modify `config.yaml.example`:

```yaml
# Core settings
nats:
  url: "nats://localhost:4222"
  user: "nats_auth_user"
  pass: "passw0rd"
  audience: "APP"
  issuer_seed: "SAXXXXXX..." # Your private key from step 1
  permissions:
    publish:
      allow:
        - "topic.>"
        - "_INBOX.>"
      deny:
        - "private.>"
    subscribe:
      allow:
        - "topic.>"
        - "_INBOX.>"
      deny:
        - "private.>"

gitlab:
  url: "https://gitlab.example"
  timeout: 5
```

#### 3. Configure NATS Server

Add to your NATS configuration:

```
authorization {  
  timeout: 2.0  
  
  auth_callout {  
    # Public key generated in step 1
    issuer: "AXXXXXXXXX..."
    # Account that will handle authorization requests  
    account: "SYS"  
    # Users allowed to handle authorization requests  
    auth_users: ["nats_auth_user"]  
  }
}

# Define accounts
accounts {
  SYS: {
    users: [
      { user: "nats_auth_user", password: "passw0rd" }
    ]
  }
  APP: {
    # This account will be used for GitLab-authenticated users
  }
}
```

## Template-Based Permissions

GCS Antal supports Go template-based permissions that dynamically adapt to the authenticated user. This provides more granular access control and security isolation between users.

### Available Template Variables

Currently supported template variables:

| Variable | Description | Example Usage |
|----------|-------------|---------------|
| `{{.Username}}` | Authenticated GitLab username | `user.{{.Username}}.>` |

### How It Works

When a user authenticates with GitLab, their username is injected into permission templates before authorizing NATS access:

```yaml
# In config.yaml
permissions:
  publish:
    allow:
      - "user.{{.Username}}.>"  # For user "john" becomes "user.john.>"
  subscribe:
    allow:
      - "user.{{.Username}}.>"  # For user "john" becomes "user.john.>"
      - "global.>"              # Unchanged - all users can access
```

## Building

Build a standalone binary:

```bash
# Build for current platform
go build -o gcs_antal

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o gcs_antal-linux-amd64

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o gcs_antal.exe
```

## Running the Service

There are multiple ways to run the service:

### Using the Binary

```bash
# Run with default config file (./config.yaml)
./gcs_antal

# Run with custom config file
./gcs_antal --config /path/to/config.yaml
```

### Using Go Directly

```bash
# Run with default config
go run main.go

# Run with custom config file
go run main.go --config /path/to/config.yaml
```

## Monitoring and Health

The service exposes HTTP endpoints for monitoring:

- **Health Check**: `GET /health` - Returns status of the service
- **Metrics**: `GET /metrics` - Prometheus metrics endpoint

These endpoints can be used with monitoring tools like Prometheus and for health checks in container orchestration systems.

## Testing

Connect to NATS using your GitLab credentials:

```bash
nats pub test.subject "hello" --user "Nick" --password "glpat-your-gitlab-token"

# Test connectivity
nats server check connection -s tls://nats.example.com --user Nick --password glpat-personal-access-token
```

## Troubleshooting

- **Authentication Failures**: Check GitLab token validity and permissions
- **Permission Issues**: Review the configured permissions in `config.yaml`
- **Connection Problems**: Verify NATS server configuration and network connectivity

## License

MIT

---

*GCS Antal is designed to be lightweight, secure, and easy to integrate into existing infrastructure.*