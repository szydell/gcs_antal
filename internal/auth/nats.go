package auth

import (
	// Import crypto/rand

	"fmt"
	"log/slog"
	"strings"
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
	logger        *slog.Logger
}

// NewNATSClient creates a new NATS client
func NewNATSClient(url, user, pass string, issuerSeed, xKeySeed string, gitlabClient *GitLabClient) (*NATSClient, error) {
	logger := slog.With("component", "nats_client")

	// Log connection parameters (without sensitive data)
	logger.Info("Attempting to connect to NATS", "url", url)

	// Parse the issuer seed
	issuerKeyPair, err := nkeys.FromSeed([]byte(issuerSeed))
	if err != nil {
		return nil, fmt.Errorf("invalid issuer seed: %w", err)
	}

	// Parse the xKey seed if provided
	var xKeyPair nkeys.KeyPair
	if xKeySeed != "" {
		xKeyPair, err = nkeys.FromSeed([]byte(xKeySeed))
		if err != nil {
			return nil, fmt.Errorf("invalid xKey seed: %w", err)
		}
	}

	// Connect to NATS with standard options
	opts := []nats.Option{
		nats.ReconnectWait(5 * time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			logger.Warn("Disconnected from NATS", "error", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("Reconnected to NATS", "server", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(nc *nats.Conn, s *nats.Subscription, err error) {
			logger.Error("NATS error", "error", err)
		}),
	}

	// Add authentication if provided
	if user != "" && pass != "" {
		opts = append(opts, nats.UserInfo(user, pass))
	}

	// Connect to NATS
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	logger.Info("Connected to NATS server", "url", nc.ConnectedUrl())

	return &NATSClient{
		nc:            nc,
		issuerKeyPair: issuerKeyPair,
		xKeyPair:      xKeyPair,
		gitlabClient:  gitlabClient,
		logger:        logger,
	}, nil
}

// Start starts listening for authentication requests
func (c *NATSClient) Start() error {
	// Subscribe to the auth_callout subject
	_, err := c.nc.Subscribe("$SYS.REQ.USER.AUTH", func(msg *nats.Msg) {
		c.handleAuthRequest(msg)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to auth requests: %w", err)
	}

	c.logger.Info("Started listening for authentication requests")
	return nil
}

// handleAuthRequest processes an authentication request from NATS
func (c *NATSClient) handleAuthRequest(msg *nats.Msg) {
	c.logger.Debug("Received auth request", "data_length", len(msg.Data))

	// Decode the authorization request claims
	rc, err := jwt.DecodeAuthorizationRequestClaims(string(msg.Data))
	if err != nil {
		c.logger.Error("Failed to decode auth request", "error", err)
		// Nie znamy userNkey ani serverId, więc wysyłamy puste
		c.respondMsg(msg.Reply, "", "", "", "invalid request format")
		return
	}

	// Wyciągnij potrzebne dane z żądania JWT
	userNkey := rc.UserNkey
	serverId := rc.Server.ID
	username := rc.ConnectOptions.Username
	token := rc.ConnectOptions.Password

	c.logger.Info("Processing auth request", "username", username, "nkey", userNkey)

	// Verify GitLab token
	authentic, err := c.gitlabClient.VerifyToken(token)
	if err != nil {
		c.logger.Error("Error verifying GitLab token", "error", err)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "authentication error")
		sentry.CaptureException(err)
		return
	}

	if !authentic {
		c.logger.Info("Authentication failed", "username", username)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "invalid credentials")
		return
	}

	// Authentication successful
	c.logger.Info("Authentication successful", "username", username)

	// Create user claims with permissions
	uc := jwt.NewUserClaims(userNkey)
	uc.Name = username

	// Use Audience from configuration
	uc.Audience = viper.GetString("nats.audience")

	// Set permissions from configuration
	// Publish permissions
	pubAllow := viper.GetStringSlice("nats.permissions.publish.allow")
	for _, subject := range pubAllow {
		uc.Permissions.Pub.Allow.Add(subject)
		c.logger.Debug("Added publish allow permission", "subject", subject)
	}

	pubDeny := viper.GetStringSlice("nats.permissions.publish.deny")
	for _, subject := range pubDeny {
		uc.Permissions.Pub.Deny.Add(subject)
		c.logger.Debug("Added publish deny permission", "subject", subject)
	}

	// Subscribe permissions
	subAllow := viper.GetStringSlice("nats.permissions.subscribe.allow")
	for _, subject := range subAllow {
		uc.Permissions.Sub.Allow.Add(subject)
		c.logger.Debug("Added subscribe allow permission", "subject", subject)
	}

	subDeny := viper.GetStringSlice("nats.permissions.subscribe.deny")
	for _, subject := range subDeny {
		uc.Permissions.Sub.Deny.Add(subject)
		c.logger.Debug("Added subscribe deny permission", "subject", subject)
	}

	// Validate the claims
	vr := jwt.CreateValidationResults()
	uc.Validate(vr)
	if len(vr.Errors()) > 0 {
		c.logger.Error("Error validating user claims", "errors", vr.Errors())
		c.respondMsg(msg.Reply, userNkey, serverId, "", fmt.Sprintf("error validating claims: %s", vr.Errors()))
		return
	}

	// Encode the user claims
	userJwt, err := uc.Encode(c.issuerKeyPair)
	if err != nil {
		c.logger.Error("Error encoding user JWT", "error", err)
		c.respondMsg(msg.Reply, userNkey, serverId, "", "error encoding user JWT")
		return
	}

	// Send response with encoded JWT - use userNkey instead of issuerPubKey
	c.respondMsg(msg.Reply, userNkey, serverId, userJwt, "")
}

// respondMsg sends an authentication response to NATS
func (c *NATSClient) respondMsg(replySubject, userNkey, serverId, userJwt, errMsg string) {
	// If userNkey is empty or invalid, generate a temporary one
	if userNkey == "" || !strings.HasPrefix(userNkey, "U") {
		c.logger.Warn("Invalid userNkey, generating temporary one", "userNkey", userNkey)
		keypair, err := nkeys.CreateUser()
		if err != nil {
			c.logger.Error("Failed to generate temporary NKey", "error", err)
			return
		}

		userNkey, err = keypair.PublicKey()
		if err != nil {
			c.logger.Error("Failed to get public key from temporary NKey", "error", err)
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
		return
	}

	data := []byte(token)

	// Send the response
	if err := c.nc.Publish(replySubject, data); err != nil {
		c.logger.Error("Failed to publish response", "error", err)
	} else {
		if errMsg == "" {
			c.logger.Debug("Sent successful auth response", "length", len(data))
		} else {
			c.logger.Debug("Sent error auth response", "length", len(data), "error", errMsg)
		}
	}
}

// Stop cleanly closes the NATS connection
func (c *NATSClient) Stop() {
	if c.nc != nil && !c.nc.IsClosed() {
		c.logger.Info("Closing NATS connection")
		c.nc.Close()
	}
}
