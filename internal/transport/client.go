// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	// API endpoints
	endpointSnapshot  = "/api/v1/agent/snapshot"
	endpointEvents    = "/api/v1/agent/events"
	endpointHeartbeat = "/api/v1/agent/heartbeat"

	// Default timeouts
	defaultTimeout = 30 * time.Second

	// Retry configuration
	maxRetries        = 5
	initialBackoff    = 1 * time.Second
	maxBackoff        = 60 * time.Second
	backoffMultiplier = 2.0
	jitterFactor      = 0.1
)

// TokenProvider provides access tokens for API authentication
type TokenProvider interface {
	// GetAccessToken returns a valid access token, refreshing if necessary
	GetAccessToken(ctx context.Context) (string, error)
}

// Client handles HTTP communication with the Obsyk platform.
// Client is safe for concurrent use by multiple goroutines.
type Client struct {
	httpClient  *http.Client
	platformURL string
	log         logr.Logger

	mu            sync.RWMutex // Protects tokenProvider
	tokenProvider TokenProvider
}

// ClientConfig holds configuration for creating a new Client.
type ClientConfig struct {
	PlatformURL   string
	TokenProvider TokenProvider
	Timeout       time.Duration
	Logger        logr.Logger
}

// NewClient creates a new transport client.
func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		platformURL:   cfg.PlatformURL,
		tokenProvider: cfg.TokenProvider,
		log:           cfg.Logger,
	}
}

// UpdateTokenProvider updates the token provider used for authentication.
// This method is safe for concurrent use.
func (c *Client) UpdateTokenProvider(provider TokenProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenProvider = provider
}

// SendSnapshot sends a full cluster state snapshot to the platform.
func (c *Client) SendSnapshot(ctx context.Context, payload *SnapshotPayload) error {
	return c.sendWithRetry(ctx, endpointSnapshot, payload)
}

// SendEvent sends a single resource change event to the platform.
func (c *Client) SendEvent(ctx context.Context, payload *EventPayload) error {
	return c.sendWithRetry(ctx, endpointEvents, payload)
}

// SendHeartbeat sends a periodic health check to the platform.
func (c *Client) SendHeartbeat(ctx context.Context, payload *HeartbeatPayload) error {
	return c.sendWithRetry(ctx, endpointHeartbeat, payload)
}

// sendWithRetry sends a request with exponential backoff retry for server errors.
func (c *Client) sendWithRetry(ctx context.Context, endpoint string, payload interface{}) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := c.calculateBackoff(attempt)
			c.log.V(1).Info("retrying request",
				"endpoint", endpoint,
				"attempt", attempt+1,
				"backoff", backoff.String())

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := c.send(ctx, endpoint, payload)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable (5xx server errors)
		if !isRetryableError(err) {
			c.log.Error(err, "non-retryable error", "endpoint", endpoint)
			return err
		}

		c.log.V(1).Info("retryable error occurred",
			"endpoint", endpoint,
			"attempt", attempt+1,
			"error", err.Error())
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// send performs the actual HTTP request.
func (c *Client) send(ctx context.Context, endpoint string, payload interface{}) error {
	// Get token provider (protected by mutex)
	c.mu.RLock()
	tokenProvider := c.tokenProvider
	c.mu.RUnlock()

	// Get access token (TokenManager is thread-safe)
	accessToken, err := tokenProvider.GetAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("getting access token: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	url := c.platformURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "obsyk-operator/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error messages
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Create appropriate error based on status code
	switch {
	case resp.StatusCode == 401:
		// Unauthorized - token may be expired or invalid
		return &AuthError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	case resp.StatusCode >= 500:
		return &ServerError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	case resp.StatusCode >= 400:
		return &ClientError{
			StatusCode: resp.StatusCode,
			Message:    string(respBody),
		}
	default:
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, respBody)
	}
}

// calculateBackoff returns the backoff duration for a given retry attempt.
func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: initialBackoff * multiplier^attempt
	backoff := float64(initialBackoff) * math.Pow(backoffMultiplier, float64(attempt))

	// Cap at max backoff
	if backoff > float64(maxBackoff) {
		backoff = float64(maxBackoff)
	}

	// Add jitter (±10%)
	jitter := backoff * jitterFactor * (2*rand.Float64() - 1)
	backoff += jitter

	return time.Duration(backoff)
}

// AuthError represents an authentication error (401).
type AuthError struct {
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("auth error %d: %s", e.StatusCode, e.Message)
}

// ServerError represents a 5xx server error (retryable).
type ServerError struct {
	StatusCode int
	Message    string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error %d: %s", e.StatusCode, e.Message)
}

// ClientError represents a 4xx client error (not retryable).
type ClientError struct {
	StatusCode int
	Message    string
}

func (e *ClientError) Error() string {
	return fmt.Sprintf("client error %d: %s", e.StatusCode, e.Message)
}

// isRetryableError returns true if the error should trigger a retry.
func isRetryableError(err error) bool {
	_, ok := err.(*ServerError)
	return ok
}
