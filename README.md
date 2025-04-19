
# NATS-GitLab Auth Service

A lightweight authentication microservice that connects NATS authentication with GitLab Personal Access Tokens.

## Overview

This service acts as a bridge between NATS `auth_callout` mechanism and GitLab's API, allowing NATS clients to authenticate using GitLab credentials.

## Setup

### Prerequisites
- Go 1.19+
- GitLab instance with API access
- NATS server (v2.10.0+) with `auth_callout` support

### Configuration
1. Copy `config.yaml.example` to `config.yaml`
2. Adjust settings as needed
3. Or use environment variables (see `.env.example`)

### Running
```bash
# Direct
go run main.go

# Docker
docker-compose up -d
```

### NATS Server Configuration
```
authorization {
  auth_callout {
    url: "http://nats-gitlab-auth:8080/auth"
    timeout: "1s"
  }
}
```

## Usage

Clients connect to NATS providing:
- Username: GitLab username
- Password: GitLab Personal Access Token

## License

MIT
