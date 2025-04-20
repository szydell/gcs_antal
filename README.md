# NATS-GitLab Auth Service

A lightweight authentication microservice that connects NATS authentication with GitLab Personal Access Tokens.

## Overview

This service acts as a bridge between NATS `auth_callout` mechanism and GitLab's API, allowing NATS clients to authenticate using GitLab credentials.

## Setup

### Prerequisites
- Go 1.23+
- GitLab instance with API access
- NATS server (v2.10.0+) with `auth_callout` support
- NATS `nk` tool for key generation

### Configuration
1. Copy `config.yaml.example` to `config.yaml`
2. Adjust settings as needed
3. Or use environment variables (see `.env.example`)

### Running
```bash
# Direct
go run main.go

```

### Generating Authentication Keys

The service requires NATS authentication keys for secure communication with the NATS server. Use the NATS `nk` tool to generate these keys:

#### Generate nseed (NATS signing key)
```bash
./nk -gen server -pubout > server.pub
./nk -inkey server.pub > nseed
```

#### Generate xseed (Encryption key)
```bash
./nk -gen x25519 -pubout > x25519.pub
./nk -inkey x25519.pub > xseed
```

Add these keys to your configuration file:
```yaml
auth:
  nseed: "YOUR_NSEED_CONTENT"
  xseed: "YOUR_XSEED_CONTENT"
```

### NATS Server Configuration

There are two approaches to configuring NATS for authentication:

#### HTTP Auth Callout (Recommended for this service)

Basic configuration (signing required):
```
authorization {
  auth_callout {
    url: "http://nats-gitlab-auth:8080/auth"
    timeout: "1s"
    
    # Required for signature verification
    signing_key_file: "/path/to/server.pub"
  }
}
```

With encryption and signing (full security):
```
authorization {
  auth_callout {
    url: "http://nats-gitlab-auth:8080/auth"
    timeout: "2s"
    
    # Keys for signature verification and encryption
    signing_key_file: "/path/to/server.pub"
    x_key_file: "/path/to/x25519.pub"
  }
}
```

The `signing_key_file` must point to the public key corresponding to the `nseed` in your service configuration.
The `x_key_file` must point to the public key corresponding to the `xseed` in your service configuration.


## Authentication Flow

1. NATS client connects with GitLab username and Personal Access Token as credentials
2. NATS server forwards these credentials to our service via `auth_callout`
3. Our service verifies the token against GitLab's API
4. Based on verification results, our service authorizes or denies access
5. The NATS server enforces the authorization decision

## Usage

Clients connect to NATS providing:
- Username: GitLab username
- Password: GitLab Personal Access Token

## Error Handling

- Invalid GitLab tokens result in authentication failure
- API timeouts and errors are logged but not exposed in responses

## License

MIT
