# GCS Antal
## NATS-GitLab Authentication Service

A lightweight authentication microservice that bridges NATS `auth_callout` authentication with GitLab Personal Access Tokens.

## What Problem Does It Solve?

GCS Antal solves the challenge of securely connecting distributed applications using NATS messaging while leveraging existing GitLab credentials. Instead of managing separate authentication systems, teams can use their GitLab Personal Access Tokens to securely access NATS messaging channels, with permissions automatically mapped to their GitLab identity.

This means:
- Users don't need to manage multiple sets of credentials
- Administrators can control access through GitLab's familiar interface
- Access to messaging topics can be automatically scoped to user identity
- Revoking GitLab access automatically revokes messaging access

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

## Getting Help and Contributing
### Reporting Issues
If you encounter bugs or have feature requests, please file an issue at: [https://github.com/szydell/gcs_antal/issues](https://github.com/szydell/gcs_antal/issues)
When reporting issues, please include:
- Steps to reproduce the problem
- Expected behavior
- Actual behavior
- GCS Antal version
- Go version
- NATS server version
- GitLab version (if relevant)

### Security Vulnerabilities
For security vulnerabilities and sensitive bug reports, please follow the process described in our [SECURITY.md](SECURITY.md) file. We take security issues seriously and appreciate your responsible disclosure.


### How to Contribute
We welcome contributions to GCS Antal! Here's how you can help:
1. **Fork the repository** on GitHub
2. **Create a branch** for your changes
3. **Make your changes** following our coding standards
4. **Add tests** for your changes
5. **Submit a pull request** with a clear description of the changes

#### Contribution Requirements
All contributions must adhere to the following standards:
- Follow the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) for style guidance
- Include tests for new functionality
- Maintain or improve code coverage
- Document new features or changes in the README
- Ensure all CI checks pass
- Include a clear description of the purpose and implementation details in your PR

### Getting Support
For questions or help using GCS Antal:
- Check existing issues on GitHub for similar questions
- Open a new discussion in the Issues section for usage questions

## License

MIT

---

*GCS Antal is designed to be lightweight, secure, and easy to integrate into existing infrastructure.*