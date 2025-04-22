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

### Response - example
```golang
	// Helper function to construct an authorization response.
	respondMsg := func(req micro.Request, userNkey, serverId, userJwt, errMsg string) {
		rc := jwt.NewAuthorizationResponseClaims(userNkey)
		rc.Audience = serverId
		rc.Error = errMsg
		rc.Jwt = userJwt

		// Sign the response with the issuer account.
		token, err := rc.Encode(issuerKeyPair)
		if err != nil {
			log.Printf("error encoding response JWT: %s", err)
			req.Respond(nil)
			return
		}

		data := []byte(token)

		// Check if encryption is required.
		xkey := req.Headers().Get("Nats-Server-Xkey")
		if len(xkey) > 0 {
			data, err = curveKeyPair.Seal(data, xkey)
			if err != nil {
				log.Printf("error encrypting response JWT: %s", err)
				req.Respond(nil)
				return
			}
		}

		log.Print("responding to authorization request")

		req.Respond(data)
	}
```

### Handling NATS `auth_callout` request - example code
// Define the message handler for the authorization request.
	msgHandler := func(req micro.Request) {
		var token []byte

		// Check for Xkey header and decrypt
		xkey := req.Headers().Get("Nats-Server-Xkey")
		if len(xkey) > 0 {
			if curveKeyPair == nil {
				respondMsg(req, "", "", "", "xkey not supported")
				return
			}

			// Decrypt the message.
			token, err = curveKeyPair.Open(req.Data(), xkey)
			if err != nil {
				respondMsg(req, "", "", "", fmt.Sprintf("error decrypting message: %s", err))
				return
			}
		} else {
			token = req.Data()
		}

		// Decode the authorization request claims.
		rc, err := jwt.DecodeAuthorizationRequestClaims(string(token))
		if err != nil {
			respondMsg(req, "", "", "", err.Error())
			return
		}

		// Used for creating the auth response.
		userNkey := rc.UserNkey
		serverId := rc.Server.ID

		// Check if the user exists.
		userProfile, ok := users[rc.ConnectOptions.Username]
		if !ok {
			respondMsg(req, userNkey, serverId, "", "user not found")
			return
		}

----->> here check Password against GitLab API

		// Prepare a user JWT.
		uc := jwt.NewUserClaims(rc.UserNkey)
		uc.Name = rc.ConnectOptions.Username

		// Check if signing key is associated, otherwise assume non-operator mode
		// and set the audience to the account.
		var sk nkeys.KeyPair
		signingKey, ok := signingKeys[userProfile.Account]
		if !ok {
			uc.Audience = userProfile.Account
		} else {
			sk = signingKey.KeyPair
			uc.IssuerAccount = signingKey.PublicKey
		}

		// Set the associated permissions if present.
		uc.Permissions = userProfile.Permissions

		// Validate the claims.
		vr := jwt.CreateValidationResults()
		uc.Validate(vr)
		if len(vr.Errors()) > 0 {
			respondMsg(req, userNkey, serverId, "", fmt.Sprintf("error validating claims: %s", vr.Errors()))
			return
		}

		// Sign it with the issuer key.
		ejwt, err := uc.Encode(sk)
		if err != nil {
			respondMsg(req, userNkey, serverId, "", fmt.Sprintf("error signing user JWT: %s", err))
			return
		}

		respondMsg(req, userNkey, serverId, ejwt, "")
	}



## Error Handling

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


# Schema
### Authorization request claims

The claims is a standard JWT structure with a nested object named nats containing the following top-level fields:

    server_id - An object describing the NATS server, include the id field needed to be used in the authorization response.

    user_nkey - A user public NKey generated by the NATS server which is used as the subject of the authorization response.

    client_info - An object describing the client attempting to connect.

    connect_opts - An object containing the data sent by client in the CONNECT message.

    client_tls - An object containing any client certificates, if applicable.
    
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "authorization-request-claims",
  "properties": {
    "aud": {
      "type": "string"
    },
    "exp": {
      "type": "integer"
    },
    "jti": {
      "type": "string"
    },
    "iat": {
      "type": "integer"
    },
    "iss": {
      "type": "string"
    },
    "name": {
      "type": "string"
    },
    "nbf": {
      "type": "integer"
    },
    "sub": {
      "type": "string"
    },
    "nats": {
      "properties": {
        "server_id": {
          "properties": {
            "name": {
              "type": "string"
            },
            "host": {
              "type": "string"
            },
            "id": {
              "type": "string"
            },
            "version": {
              "type": "string"
            },
            "cluster": {
              "type": "string"
            },
            "tags": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "xkey": {
              "type": "string"
            }
          },
          "additionalProperties": false,
          "type": "object",
          "required": [
            "name",
            "host",
            "id"
          ]
        },
        "user_nkey": {
          "type": "string"
        },
        "client_info": {
          "properties": {
            "host": {
              "type": "string"
            },
            "id": {
              "type": "integer"
            },
            "user": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "tags": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "name_tag": {
              "type": "string"
            },
            "kind": {
              "type": "string"
            },
            "type": {
              "type": "string"
            },
            "mqtt_id": {
              "type": "string"
            },
            "nonce": {
              "type": "string"
            }
          },
          "additionalProperties": false,
          "type": "object"
        },
        "connect_opts": {
          "properties": {
            "jwt": {
              "type": "string"
            },
            "nkey": {
              "type": "string"
            },
            "sig": {
              "type": "string"
            },
            "auth_token": {
              "type": "string"
            },
            "user": {
              "type": "string"
            },
            "pass": {
              "type": "string"
            },
            "name": {
              "type": "string"
            },
            "lang": {
              "type": "string"
            },
            "version": {
              "type": "string"
            },
            "protocol": {
              "type": "integer"
            }
          },
          "additionalProperties": false,
          "type": "object",
          "required": [
            "protocol"
          ]
        },
        "client_tls": {
          "properties": {
            "version": {
              "type": "string"
            },
            "cipher": {
              "type": "string"
            },
            "certs": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "verified_chains": {
              "items": {
                "items": {
                  "type": "string"
                },
                "type": "array"
              },
              "type": "array"
            }
          },
          "additionalProperties": false,
          "type": "object"
        },
        "request_nonce": {
          "type": "string"
        },
        "tags": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "type": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "additionalProperties": false,
      "type": "object",
      "required": [
        "server_id",
        "user_nkey",
        "client_info",
        "connect_opts"
      ]
    }
  },
  "additionalProperties": false,
  "type": "object",
  "required": [
    "nats"
  ]
}
```

### Authorization response claims

The claims is a standard JWT structure with a nested object named nats containing the following top-level fields:

    jwt - The encoded user claims JWT which will be used by the NATS server for the duration of the client connection.

    error - An error message sent back to the NATS server if authorization failed. This will be included log output.

    issuer_account - The public Nkey of the issuing account. If set, this indicates the claim was issued by a signing key.
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/nats-io/jwt/v2/authorization-response-claims",
  "properties": {
    "aud": {
      "type": "string"
    },
    "exp": {
      "type": "integer"
    },
    "jti": {
      "type": "string"
    },
    "iat": {
      "type": "integer"
    },
    "iss": {
      "type": "string"
    },
    "name": {
      "type": "string"
    },
    "nbf": {
      "type": "integer"
    },
    "sub": {
      "type": "string"
    },
    "nats": {
      "properties": {
        "jwt": {
          "type": "string"
        },
        "error": {
          "type": "string"
        },
        "issuer_account": {
          "type": "string"
        },
        "tags": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "type": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "additionalProperties": false,
      "type": "object"
    }
  },
  "additionalProperties": false,
  "type": "object",
  "required": [
    "nats"
  ]
}
```
### User claims

The claims is a standard JWT structure with a nested object named nats containing the following, notable, top-level fields:

    issuer_account - The public Nkey of the issuing account. If set, this indicates the claim was issued by a signing key.
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/nats-io/jwt/v2/user-claims",
  "properties": {
    "aud": {
      "type": "string"
    },
    "exp": {
      "type": "integer"
    },
    "jti": {
      "type": "string"
    },
    "iat": {
      "type": "integer"
    },
    "iss": {
      "type": "string"
    },
    "name": {
      "type": "string"
    },
    "nbf": {
      "type": "integer"
    },
    "sub": {
      "type": "string"
    },
    "nats": {
      "properties": {
        "pub": {
          "properties": {
            "allow": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "deny": {
              "items": {
                "type": "string"
              },
              "type": "array"
            }
          },
          "additionalProperties": false,
          "type": "object"
        },
        "sub": {
          "properties": {
            "allow": {
              "items": {
                "type": "string"
              },
              "type": "array"
            },
            "deny": {
              "items": {
                "type": "string"
              },
              "type": "array"
            }
          },
          "additionalProperties": false,
          "type": "object"
        },
        "resp": {
          "properties": {
            "max": {
              "type": "integer"
            },
            "ttl": {
              "type": "integer"
            }
          },
          "additionalProperties": false,
          "type": "object",
          "required": [
            "max",
            "ttl"
          ]
        },
        "src": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "times": {
          "items": {
            "properties": {
              "start": {
                "type": "string"
              },
              "end": {
                "type": "string"
              }
            },
            "additionalProperties": false,
            "type": "object"
          },
          "type": "array"
        },
        "times_location": {
          "type": "string"
        },
        "subs": {
          "type": "integer"
        },
        "data": {
          "type": "integer"
        },
        "payload": {
          "type": "integer"
        },
        "bearer_token": {
          "type": "boolean"
        },
        "allowed_connection_types": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "issuer_account": {
          "type": "string"
        },
        "tags": {
          "items": {
            "type": "string"
          },
          "type": "array"
        },
        "type": {
          "type": "string"
        },
        "version": {
          "type": "integer"
        }
      },
      "additionalProperties": false,
      "type": "object"
    }
  },
  "additionalProperties": false,
  "type": "object"
}
```