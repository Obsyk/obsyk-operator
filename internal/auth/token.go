// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// grantType is the OAuth2 grant type for JWT Bearer assertions
	grantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"

	// tokenEndpoint is the OAuth2 token endpoint path
	tokenEndpoint = "/oauth/token"

	// assertionTTL is how long the JWT assertion is valid for
	assertionTTL = 5 * time.Minute

	// tokenRefreshBuffer is how long before expiry to refresh the token
	tokenRefreshBuffer = 5 * time.Minute
)

// Credentials holds the OAuth2 client credentials
type Credentials struct {
	// ClientID is the OAuth2 client ID issued by the platform
	ClientID string

	// PrivateKey is the ECDSA P-256 private key for signing assertions
	PrivateKey *ecdsa.PrivateKey

	// PrivateKeyPEM is the raw PEM-encoded private key (used for comparison)
	PrivateKeyPEM []byte
}

// ParseCredentials parses credentials from a Secret's data
// Expected keys: "client_id" and "private_key"
func ParseCredentials(data map[string][]byte) (*Credentials, error) {
	clientIDBytes, ok := data["client_id"]
	if !ok {
		return nil, fmt.Errorf("secret missing 'client_id' key")
	}
	clientID := strings.TrimSpace(string(clientIDBytes))
	if clientID == "" {
		return nil, fmt.Errorf("client_id is empty")
	}

	privateKeyBytes, ok := data["private_key"]
	if !ok {
		return nil, fmt.Errorf("secret missing 'private_key' key")
	}

	privateKey, err := parseECDSAPrivateKey(string(privateKeyBytes))
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	return &Credentials{
		ClientID:      clientID,
		PrivateKey:    privateKey,
		PrivateKeyPEM: privateKeyBytes,
	}, nil
}

// parseECDSAPrivateKey parses a PEM-encoded ECDSA private key
func parseECDSAPrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		pkcs8Key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("failed to parse private key: %w (also tried PKCS8: %w)", err, err2)
		}
		ecdsaKey, ok := pkcs8Key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an ECDSA private key")
		}
		return ecdsaKey, nil
	}

	return key, nil
}

// TokenResponse represents the OAuth2 token response
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// TokenManager manages OAuth2 access tokens with automatic refresh
type TokenManager struct {
	platformURL string
	credentials *Credentials
	httpClient  *http.Client
	log         logr.Logger

	mu          sync.RWMutex
	accessToken string
	expiresAt   time.Time
	refreshing  bool
	refreshCh   chan struct{}
}

// NewTokenManager creates a new token manager
func NewTokenManager(platformURL string, credentials *Credentials, httpClient *http.Client, log logr.Logger) *TokenManager {
	return &TokenManager{
		platformURL: platformURL,
		credentials: credentials,
		httpClient:  httpClient,
		log:         log,
	}
}

// UpdateCredentials updates the credentials used for authentication.
// Only invalidates the cached token if credentials have actually changed.
func (tm *TokenManager) UpdateCredentials(credentials *Credentials) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Skip if credentials haven't changed
	if tm.credentials != nil &&
		tm.credentials.ClientID == credentials.ClientID &&
		bytes.Equal(tm.credentials.PrivateKeyPEM, credentials.PrivateKeyPEM) {
		return
	}

	tm.credentials = credentials
	// Invalidate current token to force refresh with new credentials
	tm.accessToken = ""
	tm.expiresAt = time.Time{}
}

// GetAccessToken returns a valid access token, refreshing if necessary
func (tm *TokenManager) GetAccessToken(ctx context.Context) (string, error) {
	// Fast path: check if we have a valid token
	tm.mu.RLock()
	if tm.accessToken != "" && time.Now().Add(tokenRefreshBuffer).Before(tm.expiresAt) {
		token := tm.accessToken
		tm.mu.RUnlock()
		return token, nil
	}
	tm.mu.RUnlock()

	// Slow path: need to refresh
	return tm.refreshToken(ctx)
}

// refreshToken obtains a new access token from the platform
func (tm *TokenManager) refreshToken(ctx context.Context) (string, error) {
	tm.mu.Lock()

	// Check again in case another goroutine already refreshed
	if tm.accessToken != "" && time.Now().Add(tokenRefreshBuffer).Before(tm.expiresAt) {
		token := tm.accessToken
		tm.mu.Unlock()
		return token, nil
	}

	// If already refreshing, wait for it
	if tm.refreshing {
		ch := tm.refreshCh
		tm.mu.Unlock()
		<-ch
		return tm.GetAccessToken(ctx)
	}

	// Mark as refreshing
	tm.refreshing = true
	tm.refreshCh = make(chan struct{})
	creds := tm.credentials
	tm.mu.Unlock()

	// Create JWT assertion
	assertion, err := tm.createAssertion(creds)
	if err != nil {
		tm.finishRefresh("", time.Time{})
		return "", fmt.Errorf("creating assertion: %w", err)
	}

	// Exchange assertion for access token
	tokenResp, err := tm.exchangeAssertion(ctx, assertion)
	if err != nil {
		tm.finishRefresh("", time.Time{})
		return "", fmt.Errorf("exchanging assertion: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	tm.finishRefresh(tokenResp.AccessToken, expiresAt)

	tm.log.Info("obtained access token",
		"expires_in", tokenResp.ExpiresIn,
		"expires_at", expiresAt.Format(time.RFC3339))

	return tokenResp.AccessToken, nil
}

// finishRefresh updates the token state and signals waiting goroutines
func (tm *TokenManager) finishRefresh(token string, expiresAt time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.accessToken = token
	tm.expiresAt = expiresAt
	tm.refreshing = false
	if tm.refreshCh != nil {
		close(tm.refreshCh)
		tm.refreshCh = nil
	}
}

// createAssertion creates a signed JWT Bearer assertion
func (tm *TokenManager) createAssertion(creds *Credentials) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		ID:        uuid.New().String(),
		Issuer:    creds.ClientID,
		Subject:   creds.ClientID,
		Audience:  jwt.ClaimStrings{tm.platformURL},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(assertionTTL)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	return token.SignedString(creds.PrivateKey)
}

// exchangeAssertion exchanges a JWT assertion for an access token
func (tm *TokenManager) exchangeAssertion(ctx context.Context, assertion string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", grantType)
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tm.platformURL+tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "obsyk-operator/1.0")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("empty access token in response")
	}

	return &tokenResp, nil
}
