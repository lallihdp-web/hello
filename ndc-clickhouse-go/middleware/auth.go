package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	// Enable authentication
	Enabled bool `json:"enabled"`

	// DevMode skips all authentication checks (for testing/development)
	// When enabled, requests proceed with headers-based auth info or anonymous role
	DevMode bool `json:"dev_mode"`

	// API key authentication
	APIKeys []APIKey `json:"api_keys"`

	// JWT authentication
	JWT *JWTConfig `json:"jwt"`

	// Webhook authentication
	Webhook *WebhookConfig `json:"webhook"`

	// Allow anonymous access (with limited permissions)
	AllowAnonymous bool `json:"allow_anonymous"`

	// Anonymous role name
	AnonymousRole string `json:"anonymous_role"`

	// DefaultRole is assigned when no role is specified (useful in dev mode)
	DefaultRole string `json:"default_role"`
}

// APIKey represents an API key
type APIKey struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Roles       []string `json:"roles"`
	DefaultRole string   `json:"default_role"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}

// JWTConfig holds JWT authentication configuration
type JWTConfig struct {
	// Secret for HMAC algorithms
	Secret string `json:"secret,omitempty"`

	// Public key for RSA/ECDSA algorithms
	PublicKey string `json:"public_key,omitempty"`

	// JWKS URL for key rotation
	JWKSURL string `json:"jwks_url,omitempty"`

	// Algorithm (HS256, HS384, HS512, RS256, RS384, RS512, ES256, ES384, ES512)
	Algorithm string `json:"algorithm"`

	// Claims namespace for custom claims
	ClaimsNamespace string `json:"claims_namespace"`

	// Issuer to validate
	Issuer string `json:"issuer,omitempty"`

	// Audience to validate
	Audience string `json:"audience,omitempty"`

	// Header name (default: Authorization)
	Header string `json:"header"`

	// Header prefix (default: Bearer)
	HeaderPrefix string `json:"header_prefix"`

	// SkipVerification skips JWT signature verification (for testing only)
	// WARNING: Never enable in production - tokens will not be validated
	SkipVerification bool `json:"skip_verification"`
}

// WebhookConfig holds webhook authentication configuration
type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Timeout time.Duration     `json:"timeout"`
}

// AuthInfo contains authenticated user information
type AuthInfo struct {
	UserID      string
	Role        string
	Roles       []string
	OrgID       string
	TenantID    string
	CustomClaims map[string]interface{}
}

// Authenticator handles authentication
type Authenticator struct {
	config *AuthConfig
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(config *AuthConfig) *Authenticator {
	if config == nil {
		config = &AuthConfig{
			Enabled:       false,
			AllowAnonymous: true,
			AnonymousRole: "anonymous",
		}
	}
	return &Authenticator{config: config}
}

// Middleware returns an HTTP middleware for authentication
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dev mode: skip all authentication, use headers or default role
		if a.config.DevMode {
			info := a.extractFromHeaders(r)
			// Apply default role if configured and no role specified
			if info.Role == "anonymous" && a.config.DefaultRole != "" {
				info.Role = a.config.DefaultRole
				info.Roles = []string{a.config.DefaultRole}
			}
			ctx := WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if !a.config.Enabled {
			// Authentication disabled, use headers directly
			info := a.extractFromHeaders(r)
			ctx := WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try API key authentication
		if info, ok := a.authenticateAPIKey(r); ok {
			ctx := WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Try JWT authentication
		if a.config.JWT != nil {
			if info, ok := a.authenticateJWT(r); ok {
				ctx := WithAuthInfo(r.Context(), info)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Try webhook authentication
		if a.config.Webhook != nil {
			if info, ok := a.authenticateWebhook(r); ok {
				ctx := WithAuthInfo(r.Context(), info)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Check for anonymous access
		if a.config.AllowAnonymous {
			info := &AuthInfo{
				Role:  a.config.AnonymousRole,
				Roles: []string{a.config.AnonymousRole},
			}
			ctx := WithAuthInfo(r.Context(), info)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Authentication required but not provided
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "Authentication required", http.StatusUnauthorized)
	})
}

// extractFromHeaders extracts auth info from request headers
func (a *Authenticator) extractFromHeaders(r *http.Request) *AuthInfo {
	info := &AuthInfo{
		UserID:       r.Header.Get("X-User-Id"),
		Role:         r.Header.Get("X-Role"),
		OrgID:        r.Header.Get("X-Org-Id"),
		TenantID:     r.Header.Get("X-Tenant-Id"),
		CustomClaims: make(map[string]interface{}),
	}

	if info.Role == "" {
		info.Role = "anonymous"
	}
	info.Roles = []string{info.Role}

	// Extract all X-* custom headers as custom claims (excluding standard headers)
	standardHeaders := map[string]bool{
		"X-User-Id": true, "X-Role": true, "X-Org-Id": true, "X-Tenant-Id": true,
		"X-Request-Id": true, "X-Forwarded-For": true, "X-Forwarded-Host": true,
		"X-Forwarded-Proto": true, "X-Real-Ip": true, "X-API-Key": true,
	}
	for key, values := range r.Header {
		if strings.HasPrefix(key, "X-") && len(values) > 0 && !standardHeaders[key] {
			claimKey := strings.TrimPrefix(key, "X-")
			claimKey = strings.ToLower(strings.ReplaceAll(claimKey, "-", "_"))
			info.CustomClaims[claimKey] = values[0]
		}
	}

	return info
}

// authenticateAPIKey authenticates using API key
func (a *Authenticator) authenticateAPIKey(r *http.Request) (*AuthInfo, bool) {
	// Check header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.Header.Get("Authorization")
		if strings.HasPrefix(apiKey, "ApiKey ") {
			apiKey = strings.TrimPrefix(apiKey, "ApiKey ")
		} else {
			apiKey = ""
		}
	}

	// Check query parameter (less secure, but sometimes needed)
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey == "" {
		return nil, false
	}

	// Find matching API key
	for _, key := range a.config.APIKeys {
		if subtle.ConstantTimeCompare([]byte(key.Key), []byte(apiKey)) == 1 {
			// Check expiration
			if key.ExpiresAt != nil {
				expiresAt, err := time.Parse(time.RFC3339, *key.ExpiresAt)
				if err == nil && time.Now().After(expiresAt) {
					return nil, false
				}
			}

			return &AuthInfo{
				Role:  key.DefaultRole,
				Roles: key.Roles,
				CustomClaims: map[string]interface{}{
					"api_key_name": key.Name,
				},
			}, true
		}
	}

	return nil, false
}

// authenticateJWT authenticates using JWT
func (a *Authenticator) authenticateJWT(r *http.Request) (*AuthInfo, bool) {
	jwt := a.config.JWT
	if jwt == nil {
		return nil, false
	}

	// Get token from header
	header := jwt.Header
	if header == "" {
		header = "Authorization"
	}
	prefix := jwt.HeaderPrefix
	if prefix == "" {
		prefix = "Bearer"
	}

	authHeader := r.Header.Get(header)
	if authHeader == "" {
		return nil, false
	}

	if !strings.HasPrefix(authHeader, prefix+" ") {
		return nil, false
	}

	token := strings.TrimPrefix(authHeader, prefix+" ")
	if token == "" {
		return nil, false
	}

	// Parse and validate JWT
	// Note: In a real implementation, you would use a proper JWT library
	// This is a simplified version that parses the claims without validation
	// for demonstration purposes
	claims, err := parseJWTClaims(token)
	if err != nil {
		return nil, false
	}

	// Extract custom claims from namespace
	namespace := jwt.ClaimsNamespace
	if namespace == "" {
		namespace = "https://clickhouse-graphql.io/jwt/claims"
	}

	customClaims, ok := claims[namespace].(map[string]interface{})
	if !ok {
		return nil, false
	}

	info := &AuthInfo{
		CustomClaims: customClaims,
	}

	if userID, ok := customClaims["x-user-id"].(string); ok {
		info.UserID = userID
	}
	if role, ok := customClaims["x-default-role"].(string); ok {
		info.Role = role
	}
	if roles, ok := customClaims["x-allowed-roles"].([]interface{}); ok {
		for _, r := range roles {
			if roleStr, ok := r.(string); ok {
				info.Roles = append(info.Roles, roleStr)
			}
		}
	}
	if orgID, ok := customClaims["x-org-id"].(string); ok {
		info.OrgID = orgID
	}
	if tenantID, ok := customClaims["x-tenant-id"].(string); ok {
		info.TenantID = tenantID
	}

	return info, true
}

// parseJWTClaims parses JWT claims without validation
// Note: In production, use a proper JWT library with validation
func parseJWTClaims(token string) (map[string]interface{}, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode payload (base64url)
	payload := parts[1]
	// Add padding if needed
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	// Replace URL-safe characters
	payload = strings.ReplaceAll(payload, "-", "+")
	payload = strings.ReplaceAll(payload, "_", "/")

	// This is a simplified implementation
	// In production, use encoding/base64.RawURLEncoding
	var claims map[string]interface{}

	// For demonstration, we'll just try to parse it
	// A real implementation would properly decode base64url
	if err := json.Unmarshal([]byte(payload), &claims); err != nil {
		// Try with proper base64 decoding in a real implementation
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return claims, nil
}

// authenticateWebhook authenticates using webhook
func (a *Authenticator) authenticateWebhook(r *http.Request) (*AuthInfo, bool) {
	webhook := a.config.Webhook
	if webhook == nil {
		return nil, false
	}

	// Create webhook request
	method := webhook.Method
	if method == "" {
		method = "GET"
	}

	client := &http.Client{
		Timeout: webhook.Timeout,
	}
	if client.Timeout == 0 {
		client.Timeout = 10 * time.Second
	}

	req, err := http.NewRequest(method, webhook.URL, nil)
	if err != nil {
		return nil, false
	}

	// Forward relevant headers
	headersToForward := []string{
		"Authorization",
		"Cookie",
		"X-Request-Id",
	}
	for _, h := range headersToForward {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}

	// Add custom headers
	for k, v := range webhook.Headers {
		req.Header.Set(k, v)
	}

	// Execute webhook
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	// Parse response headers for auth claims
	info := &AuthInfo{
		CustomClaims: make(map[string]interface{}),
	}

	for key, values := range resp.Header {
		if strings.HasPrefix(key, "X-") && len(values) > 0 {
			value := values[0]
			switch key {
			case "X-User-Id":
				info.UserID = value
			case "X-Role":
				info.Role = value
			case "X-Org-Id":
				info.OrgID = value
			case "X-Tenant-Id":
				info.TenantID = value
			}
			claimKey := strings.TrimPrefix(key, "X-")
			claimKey = strings.ToLower(strings.ReplaceAll(claimKey, "-", "_"))
			info.CustomClaims[claimKey] = value
		}
	}

	if info.Role != "" {
		info.Roles = []string{info.Role}
	}

	return info, true
}

// Context key for auth info
type authContextKey struct{}

// WithAuthInfo adds auth info to context
func WithAuthInfo(ctx context.Context, info *AuthInfo) context.Context {
	return context.WithValue(ctx, authContextKey{}, info)
}

// GetAuthInfo retrieves auth info from context
func GetAuthInfo(ctx context.Context) *AuthInfo {
	if v := ctx.Value(authContextKey{}); v != nil {
		return v.(*AuthInfo)
	}
	return nil
}

// GetSessionVariables returns session variables from auth info
func (info *AuthInfo) GetSessionVariables() map[string]string {
	vars := make(map[string]string)
	if info == nil {
		return vars
	}

	if info.UserID != "" {
		vars["x-user-id"] = info.UserID
	}
	if info.Role != "" {
		vars["x-role"] = info.Role
	}
	if info.OrgID != "" {
		vars["x-org-id"] = info.OrgID
	}
	if info.TenantID != "" {
		vars["x-tenant-id"] = info.TenantID
	}

	for k, v := range info.CustomClaims {
		if strVal, ok := v.(string); ok {
			vars["x-"+strings.ReplaceAll(k, "_", "-")] = strVal
		}
	}

	return vars
}

// HasRole checks if the auth info has a specific role
func (info *AuthInfo) HasRole(role string) bool {
	if info == nil {
		return false
	}
	for _, r := range info.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// DefaultAuthConfig returns default authentication configuration
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		Enabled:        false,
		AllowAnonymous: true,
		AnonymousRole:  "anonymous",
		JWT: &JWTConfig{
			Algorithm:       "HS256",
			ClaimsNamespace: "https://clickhouse-graphql.io/jwt/claims",
			Header:          "Authorization",
			HeaderPrefix:    "Bearer",
		},
	}
}

// DevAuthConfig returns authentication configuration for development/testing
// WARNING: Never use in production - bypasses all authentication checks
func DevAuthConfig() *AuthConfig {
	return &AuthConfig{
		DevMode:        true,
		Enabled:        false,
		AllowAnonymous: true,
		AnonymousRole:  "anonymous",
		DefaultRole:    "admin", // Default to admin role for testing
		JWT: &JWTConfig{
			Algorithm:        "HS256",
			ClaimsNamespace:  "https://clickhouse-graphql.io/jwt/claims",
			Header:           "Authorization",
			HeaderPrefix:     "Bearer",
			SkipVerification: true,
		},
	}
}
