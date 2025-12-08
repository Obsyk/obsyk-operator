// Copyright (c) Obsyk. All rights reserved.
// Licensed under the Apache License, Version 2.0.

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
)

// generateTestKeyPair generates a test ECDSA P-256 key pair
func generateTestKeyPair(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Encode private key to PEM
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	pemBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyBytes,
	})

	return privateKey, string(pemBlock)
}

func TestParseCredentials(t *testing.T) {
	_, privatePEM := generateTestKeyPair(t)

	tests := []struct {
		name    string
		data    map[string][]byte
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid credentials",
			data: map[string][]byte{
				"client_id":   []byte("test-client-id"),
				"private_key": []byte(privatePEM),
			},
			wantErr: false,
		},
		{
			name: "missing client_id",
			data: map[string][]byte{
				"private_key": []byte(privatePEM),
			},
			wantErr: true,
			errMsg:  "missing 'client_id'",
		},
		{
			name: "missing private_key",
			data: map[string][]byte{
				"client_id": []byte("test-client-id"),
			},
			wantErr: true,
			errMsg:  "missing 'private_key'",
		},
		{
			name: "empty client_id",
			data: map[string][]byte{
				"client_id":   []byte("  "),
				"private_key": []byte(privatePEM),
			},
			wantErr: true,
			errMsg:  "client_id is empty",
		},
		{
			name: "invalid private_key",
			data: map[string][]byte{
				"client_id":   []byte("test-client-id"),
				"private_key": []byte("invalid-key"),
			},
			wantErr: true,
			errMsg:  "failed to parse PEM block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creds, err := ParseCredentials(tt.data)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if creds == nil {
					t.Error("Expected credentials but got nil")
				} else if creds.ClientID != "test-client-id" {
					t.Errorf("ClientID = %q, want %q", creds.ClientID, "test-client-id")
				}
			}
		})
	}
}

func TestTokenManager_CreateAssertion(t *testing.T) {
	privateKey, privatePEM := generateTestKeyPair(t)

	creds, err := ParseCredentials(map[string][]byte{
		"client_id":   []byte("test-client-id"),
		"private_key": []byte(privatePEM),
	})
	if err != nil {
		t.Fatalf("Failed to parse credentials: %v", err)
	}

	tm := NewTokenManager("https://api.example.com", creds, http.DefaultClient, logr.Discard())

	assertion, err := tm.createAssertion(creds)
	if err != nil {
		t.Fatalf("Failed to create assertion: %v", err)
	}

	// Verify the assertion can be parsed and validated
	token, err := jwt.ParseWithClaims(assertion, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		return &privateKey.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("Failed to parse assertion: %v", err)
	}

	if !token.Valid {
		t.Error("Token is not valid")
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		t.Fatal("Failed to get claims")
	}

	if claims.Issuer != "test-client-id" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "test-client-id")
	}

	if claims.Subject != "test-client-id" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "test-client-id")
	}

	if claims.ExpiresAt == nil {
		t.Error("ExpiresAt is nil")
	} else if claims.ExpiresAt.Before(time.Now()) {
		t.Error("Token is already expired")
	}
}

func TestTokenManager_GetAccessToken(t *testing.T) {
	_, privatePEM := generateTestKeyPair(t)

	// Create a mock token server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			t.Errorf("Unexpected method: %s", r.Method)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded" {
			t.Errorf("Unexpected Content-Type: %s", contentType)
		}

		err := r.ParseForm()
		if err != nil {
			t.Errorf("Failed to parse form: %v", err)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		grantType := r.FormValue("grant_type")
		if grantType != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("Unexpected grant_type: %s", grantType)
		}

		assertion := r.FormValue("assertion")
		if assertion == "" {
			t.Error("Missing assertion")
			http.Error(w, "Missing assertion", http.StatusBadRequest)
			return
		}

		// Return a mock token
		resp := TokenResponse{
			AccessToken: "mock-access-token-12345",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds, err := ParseCredentials(map[string][]byte{
		"client_id":   []byte("test-client-id"),
		"private_key": []byte(privatePEM),
	})
	if err != nil {
		t.Fatalf("Failed to parse credentials: %v", err)
	}

	tm := NewTokenManager(server.URL, creds, server.Client(), logr.Discard())

	// Get access token
	token, err := tm.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("Failed to get access token: %v", err)
	}

	if token != "mock-access-token-12345" {
		t.Errorf("Token = %q, want %q", token, "mock-access-token-12345")
	}

	// Second call should return cached token
	token2, err := tm.GetAccessToken(context.Background())
	if err != nil {
		t.Fatalf("Failed to get cached access token: %v", err)
	}

	if token2 != token {
		t.Errorf("Cached token = %q, want %q", token2, token)
	}
}

func TestTokenManager_TokenRefresh(t *testing.T) {
	_, privatePEM := generateTestKeyPair(t)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := TokenResponse{
			AccessToken: "token-" + string(rune('A'+callCount-1)),
			TokenType:   "Bearer",
			ExpiresIn:   1, // 1 second expiry
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds, _ := ParseCredentials(map[string][]byte{
		"client_id":   []byte("test-client-id"),
		"private_key": []byte(privatePEM),
	})

	tm := NewTokenManager(server.URL, creds, server.Client(), logr.Discard())

	// First call
	token1, _ := tm.GetAccessToken(context.Background())

	// Wait for token to expire (plus buffer)
	time.Sleep(2 * time.Second)

	// Second call should refresh
	token2, _ := tm.GetAccessToken(context.Background())

	if token1 == token2 {
		t.Error("Expected different tokens after refresh")
	}

	if callCount < 2 {
		t.Errorf("Expected at least 2 token requests, got %d", callCount)
	}
}

func TestTokenManager_UpdateCredentials(t *testing.T) {
	_, privatePEM1 := generateTestKeyPair(t)
	_, privatePEM2 := generateTestKeyPair(t)

	tokenIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenIndex++
		resp := TokenResponse{
			AccessToken: "token-" + string(rune('0'+tokenIndex)),
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds1, _ := ParseCredentials(map[string][]byte{
		"client_id":   []byte("client-1"),
		"private_key": []byte(privatePEM1),
	})

	creds2, _ := ParseCredentials(map[string][]byte{
		"client_id":   []byte("client-2"),
		"private_key": []byte(privatePEM2),
	})

	tm := NewTokenManager(server.URL, creds1, server.Client(), logr.Discard())

	// Get token with first credentials
	token1, _ := tm.GetAccessToken(context.Background())

	// Update credentials
	tm.UpdateCredentials(creds2)

	// Get token with new credentials (should refresh)
	token2, _ := tm.GetAccessToken(context.Background())

	if token1 == token2 {
		t.Error("Expected different token after credentials update")
	}
}

func TestTokenManager_UpdateCredentials_NoThrashing(t *testing.T) {
	_, privatePEM := generateTestKeyPair(t)

	tokenIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenIndex++
		resp := TokenResponse{
			AccessToken: "token-" + string(rune('0'+tokenIndex)),
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	creds, _ := ParseCredentials(map[string][]byte{
		"client_id":   []byte("test-client-id"),
		"private_key": []byte(privatePEM),
	})

	tm := NewTokenManager(server.URL, creds, server.Client(), logr.Discard())

	// Get initial token
	token1, _ := tm.GetAccessToken(context.Background())
	if tokenIndex != 1 {
		t.Fatalf("Expected 1 token request, got %d", tokenIndex)
	}

	// Update with same credentials (should NOT invalidate token)
	sameCreds, _ := ParseCredentials(map[string][]byte{
		"client_id":   []byte("test-client-id"),
		"private_key": []byte(privatePEM),
	})
	tm.UpdateCredentials(sameCreds)

	// Get token again - should return cached token, not refresh
	token2, _ := tm.GetAccessToken(context.Background())

	if token1 != token2 {
		t.Error("Expected same token when credentials unchanged")
	}

	if tokenIndex != 1 {
		t.Errorf("Expected no additional token requests, but got %d total", tokenIndex)
	}
}
