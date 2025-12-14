package auth

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"text/template"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/spf13/viper"
)

// NATSClient handles NATS authentication requests
type NATSClient struct {
	nc            *nats.Conn
	issuerKeyPair nkeys.KeyPair
	xKeyPair      nkeys.KeyPair // May be nil if not using encryption
	gitlabClient  *GitLabClient
	tokenCache    TokenCache
	logger        *slog.Logger
}

// NewNATSClient creates a new NATS client
func NewNATSClient(url, user, pass string, issuerSeed, xKeySeed string, gitlabClient *GitLabClient) (*NATSClient, error) {
	logger := slog.With("component", "nats_client")

	// Log connection parameters (without sensitive data)
	logger.Info("Attempting to connect to NATS", "url", url)

	// Add Sentry breadcrumb for connection attempt
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category: "nats",
		Message:  "Attempting to connect to NATS",
		Level:    sentry.LevelInfo,
		Data: map[string]interface{}{
			"url": url,
		},
	})

	// Parse the issuer seed
	issuerKeyPair, err := nkeys.FromSeed([]byte(issuerSeed))
	if err != nil {
		sentry.CaptureException(fmt.Errorf("invalid issuer seed: %w", err))
		return nil, fmt.Errorf("invalid issuer seed: %w", err)
	}

	// Parse the xKey seed if provided
	var xKeyPair nkeys.KeyPair
	if xKeySeed != "" {
		xKeyPair, err = nkeys.FromSeed([]byte(xKeySeed))
		if err != nil {
			sentry.CaptureException(fmt.Errorf("invalid xKey seed: %w", err))
			return nil, fmt.Errorf("invalid xKey seed: %w", err)
		}
	}

	// Connect to NATS with standard options
	opts := []nats.Option{
		nats.ReconnectWait(5 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Warn("Disconnected from NATS", "error", err)
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("connection_event", "disconnect")
				scope.SetLevel(sentry.LevelWarning)
				sentry.CaptureMessage("Disconnected from NATS server")
			})
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("Reconnected to NATS", "server", nc.ConnectedUrl())
			sentry.AddBreadcrumb(&sentry.Breadcrumb{
				Category: "nats",
				Message:  "Reconnected to NATS server",
				Level:    sentry.LevelInfo,
				Data: map[string]interface{}{
					"server": nc.ConnectedUrl(),
				},
			})
		}),
		nats.ErrorHandler(func(nc *nats.Conn, s *nats.Subscription, err error) {
			logger.Error("NATS error", "error", err)
			sentry.WithScope(func(scope *sentry.Scope) {
				scope.SetTag("error_type", "nats_subscription")
				if s != nil {
					scope.SetTag("subject", s.Subject)
				}
				sentry.CaptureException(err)
			})
		}),
	}

	// Add authentication if provided
	if user != "" && pass != "" {
		opts = append(opts, nats.UserInfo(user, pass))
	}

	// Connect to NATS
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		sentry.CaptureException(fmt.Errorf("failed to connect to NATS: %w", err))
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	logger.Info("Connected to NATS server", "url", nc.ConnectedUrl())
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category: "nats",
		Message:  "Connected to NATS server",
		Level:    sentry.LevelInfo,
		Data: map[string]interface{}{
			"server": nc.ConnectedUrl(),
		},
	})

	client := &NATSClient{
		nc:            nc,
		issuerKeyPair: issuerKeyPair,
		xKeyPair:      xKeyPair,
		gitlabClient:  gitlabClient,
		logger:        logger,
	}

	// Optional: initialize JetStream KV token cache.
	cacheCfg := LoadTokenCacheConfig()
	if cacheCfg.Enabled {
		logger.Info("Token cache config loaded (JetStream KV)",
			"enabled", cacheCfg.Enabled,
			"bucket", cacheCfg.Bucket,
			"ttl", cacheCfg.TTL,
			"replicas", cacheCfg.Replicas,
			"hmac_secret_set", cacheCfg.HMACSecret != "",
		)

		js, err := nc.JetStream()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize JetStream: %w", err)
		}
		logger.Info("JetStream initialized")
		cache, err := NewJetStreamTokenCache(js, cacheCfg)
		if err != nil {
			return nil, err
		}
		client.tokenCache = cache
		logger.Info("Token cache enabled (JetStream KV)",
			"bucket", cacheCfg.Bucket,
			"ttl", cacheCfg.TTL,
			"replicas", cacheCfg.Replicas,
		)
	} else {
		logger.Info("Token cache disabled (JetStream KV)",
			"enabled", cacheCfg.Enabled,
			"bucket", cacheCfg.Bucket,
			"ttl", cacheCfg.TTL,
			"replicas", cacheCfg.Replicas,
			"hmac_secret_set", cacheCfg.HMACSecret != "",
		)
	}

	return client, nil
}

// Start starts listening for authentication requests
func (c *NATSClient) Start() error {
	// Start Sentry transaction for NATS subscription
	ctx := context.Background()
	span := sentry.StartTransaction(ctx, "nats.subscribe.$SYS.REQ.USER.AUTH")
	defer span.Finish()

	// Subscribe to the auth_callout subject
	_, err := c.nc.Subscribe("$SYS.REQ.USER.AUTH", func(msg *nats.Msg) {
		c.handleAuthRequest(msg)
	})
	if err != nil {
		sentry.CaptureException(fmt.Errorf("failed to subscribe to auth requests: %w", err))
		return fmt.Errorf("failed to subscribe to auth requests: %w", err)
	}

	c.logger.Info("Started listening for authentication requests")
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category: "nats",
		Message:  "Started listening for authentication requests",
		Level:    sentry.LevelInfo,
	})

	return nil
}

// handleAuthRequest processes an authentication request from NATS
func (c *NATSClient) handleAuthRequest(msg *nats.Msg) {
	// Start Sentry transaction for auth request
	ctx := context.Background()
	tx := sentry.StartTransaction(ctx, "auth.request")
	defer tx.Finish()

	c.logger.Debug("Received auth request", "data_length", len(msg.Data))

	// Decode the authorization request claims
	rc, err := jwt.DecodeAuthorizationRequestClaims(string(msg.Data))
	if err != nil {
		c.logger.Error("Failed to decode auth request", "error", err)
		// Nie znamy userNkey ani serverId, więc wysyłamy puste
		c.respondMsg(msg.Reply, "", "", "", "invalid request format")

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("error_type", "decode_auth_request")
			scope.SetExtra("data_length", len(msg.Data))
			sentry.CaptureException(err)
		})
		return
	}

	// Wyciągnij potrzebne dane z żądania JWT
	userNkey := rc.UserNkey
	serverId := rc.Server.ID
	username := rc.ConnectOptions.Username
	token := rc.ConnectOptions.Password

	// Add context to Sentry transaction
	tx.SetTag("username", username)
	tx.SetTag("server_id", serverId)

	c.logger.Info("Processing auth request", "username", username)

	// Create child span for GitLab verification
	gitlabCtx := sentry.SetHubOnContext(ctx, sentry.CurrentHub())
	span := sentry.StartSpan(gitlabCtx, "auth.authorize_token")

	result, err := AuthorizeToken(ctx, token, c.gitlabClient, c.tokenCache, time.Now)
	if err != nil {
		c.logger.Error("Error authorizing token", "error", err)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "authentication error")

		span.Status = sentry.SpanStatusInternalError
		span.SetData("error", err.Error())
		span.Finish()

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetUser(sentry.User{Username: username})
			scope.SetTag("error_type", "authorize_token")
			sentry.CaptureException(err)
		})
		return
	}
	if result.CacheWriteErr != nil {
		c.logger.Warn("Failed to write token cache", "error", result.CacheWriteErr)
	}
	span.Finish()

	if !result.Allow {
		c.logger.Info("Authentication failed", "username", username)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "invalid credentials")

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetUser(sentry.User{Username: username})
			scope.SetTag("auth_status", "failed")
			scope.SetLevel(sentry.LevelWarning)
			sentry.CaptureMessage("Authentication failed - invalid credentials")
		})
		return
	}

	if result.FromCache {
		tx.SetTag("auth_source", "cache")
	} else {
		tx.SetTag("auth_source", "gitlab")
	}

	// Authentication successful
	c.logger.Info("Authentication successful", "username", username)
	tx.SetTag("auth_status", "success")

	// Create span for JWT creation
	jwtCtx := sentry.SetHubOnContext(ctx, sentry.CurrentHub())
	jwtSpan := sentry.StartSpan(jwtCtx, "jwt.create_user_claims")

	// Create user claims with permissions
	uc := jwt.NewUserClaims(userNkey)
	uc.Name = username

	// Use Audience from configuration
	uc.Audience = viper.GetString("nats.audience")

	// Set permissions from configuration
	// Publish permissions
	pubAllow := viper.GetStringSlice("nats.permissions.publish.allow")
	for _, subject := range pubAllow {
		processedSubject := c.processPermissionTemplate(subject, username)
		uc.Permissions.Pub.Allow.Add(processedSubject)
		c.logger.Debug("Added publish allow permission", "subject", processedSubject)
	}

	pubDeny := viper.GetStringSlice("nats.permissions.publish.deny")
	for _, subject := range pubDeny {
		processedSubject := c.processPermissionTemplate(subject, username)
		uc.Permissions.Pub.Deny.Add(processedSubject)
		c.logger.Debug("Added publish deny permission", "subject", processedSubject)
	}

	// Subscribe permissions
	subAllow := viper.GetStringSlice("nats.permissions.subscribe.allow")
	for _, subject := range subAllow {
		processedSubject := c.processPermissionTemplate(subject, username)
		uc.Permissions.Sub.Allow.Add(processedSubject)
		c.logger.Debug("Added subscribe allow permission", "subject", processedSubject)
	}

	subDeny := viper.GetStringSlice("nats.permissions.subscribe.deny")
	for _, subject := range subDeny {
		processedSubject := c.processPermissionTemplate(subject, username)
		uc.Permissions.Sub.Deny.Add(processedSubject)
		c.logger.Debug("Added subscribe deny permission", "subject", processedSubject)
	}
	jwtSpan.Finish()

	// Validate the claims
	valCtx := sentry.SetHubOnContext(ctx, sentry.CurrentHub())
	validationSpan := sentry.StartSpan(valCtx, "jwt.validate_claims")
	vr := jwt.CreateValidationResults()
	uc.Validate(vr)
	validationSpan.Finish()

	if len(vr.Errors()) > 0 {
		c.logger.Error("Error validating user claims", "errors", vr.Errors())
		c.respondMsg(msg.Reply, userNkey, serverId, "", fmt.Sprintf("error validating claims: %s", vr.Errors()))

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetUser(sentry.User{Username: username})
			scope.SetTag("error_type", "claim_validation")
			scope.SetExtra("validation_errors", vr.Errors())
			sentry.CaptureMessage("Error validating user claims")
		})
		return
	}

	// Encode the user claims
	encodeCtx := sentry.SetHubOnContext(ctx, sentry.CurrentHub())
	encodeSpan := sentry.StartSpan(encodeCtx, "jwt.encode_claims")
	userJwt, err := uc.Encode(c.issuerKeyPair)
	encodeSpan.Finish()

	if err != nil {
		c.logger.Error("Error encoding user JWT", "error", err)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "error encoding user JWT")

		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetUser(sentry.User{Username: username})
			scope.SetTag("error_type", "jwt_encoding")
			sentry.CaptureException(err)
		})
		return
	}

	// Send response with encoded JWT - use userNkey instead of issuerPubKey
	responseCtx := sentry.SetHubOnContext(ctx, sentry.CurrentHub())
	responseSpan := sentry.StartSpan(responseCtx, "nats.send_response")
	c.respondMsg(msg.Reply, userNkey, serverId, userJwt, "")
	responseSpan.Finish()

	// Add successful authentication metric to Sentry
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Category: "auth",
		Message:  "User successfully authenticated",
		Level:    sentry.LevelInfo,
		Data: map[string]interface{}{
			"username": username,
		},
	})
}

// processPermissionTemplate processes Go template strings in permission subjects
func (c *NATSClient) processPermissionTemplate(subjectTemplate string, username string) string {
	// Define template data structure
	type TemplateData struct {
		Username string
	}

	// Create template
	tmpl, err := template.New("permission").Parse(subjectTemplate)
	if err != nil {
		// Log error but return original string if template is invalid
		c.logger.Error("Invalid permission template", "template", subjectTemplate, "error", err)
		return subjectTemplate
	}

	// Prepare data for template
	data := TemplateData{
		Username: username,
	}

	// Execute template
	var result bytes.Buffer
	if err := tmpl.Execute(&result, data); err != nil {
		c.logger.Error("Failed to process permission template", "template", subjectTemplate, "error", err)
		return subjectTemplate
	}

	processed := result.String()
	if processed != subjectTemplate {
		c.logger.Debug("Processed permission template", "original", subjectTemplate, "processed", processed)
	}

	return processed
}

// respondMsg sends an authentication response to NATS
func (c *NATSClient) respondMsg(replySubject, userNkey, serverId, userJwt, errMsg string) {
	// If userNkey is empty or invalid, generate a temporary one
	if userNkey == "" || !strings.HasPrefix(userNkey, "U") {
		c.logger.Warn("Invalid userNkey, generating temporary one", "userNkey", userNkey)

		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Category: "auth",
			Message:  "Invalid userNkey, generating temporary one",
			Level:    sentry.LevelWarning,
			Data: map[string]interface{}{
				"userNkey": userNkey,
			},
		})

		keypair, err := nkeys.CreateUser()
		if err != nil {
			c.logger.Error("Failed to generate temporary NKey", "error", err)
			sentry.CaptureException(err)
			return
		}

		userNkey, err = keypair.PublicKey()
		if err != nil {
			c.logger.Error("Failed to get public key from temporary NKey", "error", err)
			sentry.CaptureException(err)
			return
		}
	}

	// Create authorization response claims
	rc := jwt.NewAuthorizationResponseClaims(userNkey)
	if serverId != "" {
		rc.Audience = serverId
	}
	rc.Error = errMsg
	rc.Jwt = userJwt

	// Sign with the issuer key
	token, err := rc.Encode(c.issuerKeyPair)
	if err != nil {
		c.logger.Error("Failed to encode response JWT", "error", err)
		sentry.CaptureException(err)
		return
	}

	data := []byte(token)

	// Send the response
	if err := c.nc.Publish(replySubject, data); err != nil {
		c.logger.Error("Failed to publish response", "error", err)
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("error_type", "nats_publish")
			scope.SetTag("reply_subject", replySubject)
			sentry.CaptureException(err)
		})
	} else {
		if errMsg == "" {
			c.logger.Debug("Sent successful auth response", "length", len(data))
		} else {
			c.logger.Debug("Sent error auth response", "length", len(data), "error", errMsg)
			sentry.AddBreadcrumb(&sentry.Breadcrumb{
				Category: "auth",
				Message:  "Sent error auth response",
				Level:    sentry.LevelError,
				Data: map[string]interface{}{
					"error": errMsg,
				},
			})
		}
	}
}

// Stop cleanly closes the NATS connection
func (c *NATSClient) Stop() {
	if c.nc != nil && !c.nc.IsClosed() {
		c.logger.Info("Closing NATS connection")
		sentry.AddBreadcrumb(&sentry.Breadcrumb{
			Category: "nats",
			Message:  "Closing NATS connection",
			Level:    sentry.LevelInfo,
		})
		c.nc.Close()
	}
}
