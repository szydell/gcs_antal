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

## License

MIT
