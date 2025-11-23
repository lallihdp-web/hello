package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewAuthenticator(t *testing.T) {
	config := DefaultAuthConfig()
	auth := NewAuthenticator(config)

	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

func TestAuthenticator_Disabled(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: false,
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := GetAuthInfo(r.Context())
		if info == nil {
			t.Error("expected auth info even when disabled")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthenticator_ExtractHeaders(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: false,
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := GetAuthInfo(r.Context())
		if info == nil {
			t.Fatal("expected auth info")
		}
		if info.UserID != "123" {
			t.Errorf("expected user id 123, got %s", info.UserID)
		}
		if info.Role != "admin" {
			t.Errorf("expected role admin, got %s", info.Role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Hasura-User-Id", "123")
	req.Header.Set("X-Hasura-Role", "admin")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthenticator_APIKey(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: true,
		APIKeys: []APIKey{
			{
				Key:         "test-api-key-123",
				Name:        "Test Key",
				Roles:       []string{"admin", "user"},
				DefaultRole: "admin",
			},
		},
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := GetAuthInfo(r.Context())
		if info == nil {
			t.Fatal("expected auth info")
		}
		if info.Role != "admin" {
			t.Errorf("expected role admin, got %s", info.Role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Test with X-API-Key header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-api-key-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthenticator_InvalidAPIKey(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: true,
		APIKeys: []APIKey{
			{
				Key:         "valid-key",
				Name:        "Valid",
				DefaultRole: "user",
			},
		},
		AllowAnonymous: false,
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticator_AllowAnonymous(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled:        true,
		AllowAnonymous: true,
		AnonymousRole:  "guest",
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := GetAuthInfo(r.Context())
		if info == nil {
			t.Fatal("expected auth info")
		}
		if info.Role != "guest" {
			t.Errorf("expected role guest, got %s", info.Role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAuthInfo_GetSessionVariables(t *testing.T) {
	info := &AuthInfo{
		UserID:   "123",
		Role:     "admin",
		OrgID:    "org-1",
		TenantID: "tenant-1",
		CustomClaims: map[string]interface{}{
			"department": "engineering",
		},
	}

	vars := info.GetSessionVariables()

	if vars["x-hasura-user-id"] != "123" {
		t.Errorf("expected user-id 123, got %s", vars["x-hasura-user-id"])
	}
	if vars["x-hasura-role"] != "admin" {
		t.Errorf("expected role admin, got %s", vars["x-hasura-role"])
	}
	if vars["x-hasura-org-id"] != "org-1" {
		t.Errorf("expected org-id org-1, got %s", vars["x-hasura-org-id"])
	}
	if vars["x-hasura-department"] != "engineering" {
		t.Errorf("expected department engineering, got %s", vars["x-hasura-department"])
	}
}

func TestAuthInfo_HasRole(t *testing.T) {
	info := &AuthInfo{
		Role:  "admin",
		Roles: []string{"admin", "user", "editor"},
	}

	if !info.HasRole("admin") {
		t.Error("expected HasRole(admin) to return true")
	}
	if !info.HasRole("user") {
		t.Error("expected HasRole(user) to return true")
	}
	if info.HasRole("superadmin") {
		t.Error("expected HasRole(superadmin) to return false")
	}
}

func TestAuthInfo_HasRole_Nil(t *testing.T) {
	var info *AuthInfo

	if info.HasRole("admin") {
		t.Error("expected nil AuthInfo.HasRole to return false")
	}
}

func TestWithAuthInfo(t *testing.T) {
	info := &AuthInfo{
		UserID: "test-user",
		Role:   "admin",
	}

	ctx := WithAuthInfo(context.Background(), info)
	retrieved := GetAuthInfo(ctx)

	if retrieved == nil {
		t.Fatal("expected to retrieve auth info from context")
	}
	if retrieved.UserID != "test-user" {
		t.Errorf("expected user id test-user, got %s", retrieved.UserID)
	}
}

func TestGetAuthInfo_NoInfo(t *testing.T) {
	ctx := context.Background()
	info := GetAuthInfo(ctx)

	if info != nil {
		t.Error("expected nil auth info for empty context")
	}
}

func TestAuthenticator_APIKeyAuthorizationHeader(t *testing.T) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: true,
		APIKeys: []APIKey{
			{
				Key:         "my-secret-key",
				Name:        "Test",
				DefaultRole: "user",
			},
		},
	})

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := GetAuthInfo(r.Context())
		if info == nil || info.Role != "user" {
			t.Error("expected auth info with user role")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "ApiKey my-secret-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// Benchmarks
func BenchmarkAuthenticator_Middleware_Disabled(b *testing.B) {
	auth := NewAuthenticator(&AuthConfig{Enabled: false})
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkAuthenticator_Middleware_APIKey(b *testing.B) {
	auth := NewAuthenticator(&AuthConfig{
		Enabled: true,
		APIKeys: []APIKey{
			{Key: "test-key", Name: "Test", DefaultRole: "user"},
		},
	})
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkAuthenticator_Middleware_Headers(b *testing.B) {
	auth := NewAuthenticator(&AuthConfig{Enabled: false})
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Hasura-User-Id", "123")
	req.Header.Set("X-Hasura-Role", "admin")
	req.Header.Set("X-Hasura-Org-Id", "org-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkAuthInfo_GetSessionVariables(b *testing.B) {
	info := &AuthInfo{
		UserID:   "123",
		Role:     "admin",
		OrgID:    "org-1",
		TenantID: "tenant-1",
		CustomClaims: map[string]interface{}{
			"department": "engineering",
			"team":       "backend",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info.GetSessionVariables()
	}
}
