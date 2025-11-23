package config

import (
	"testing"
)

func TestPermissionChecker_IsAdmin(t *testing.T) {
	cfg := &PermissionsConfig{
		AdminRole:   "admin",
		DefaultRole: "user",
		Roles:       make(map[string]RolePermissions),
	}

	tests := []struct {
		role     string
		expected bool
	}{
		{"admin", true},
		{"user", false},
		{"anonymous", false},
		{"", false},
	}

	for _, tt := range tests {
		pc := NewPermissionChecker(cfg, tt.role, nil)
		result := pc.IsAdmin()
		if result != tt.expected {
			t.Errorf("IsAdmin() for role %q = %v, want %v", tt.role, result, tt.expected)
		}
	}
}

func TestPermissionChecker_CanSelect(t *testing.T) {
	cfg := &PermissionsConfig{
		AdminRole:   "admin",
		DefaultRole: "anonymous",
		Roles: map[string]RolePermissions{
			"admin": {
				Tables: map[string]TablePermission{
					"users": {Select: &SelectPermission{}},
				},
			},
			"user": {
				Tables: map[string]TablePermission{
					"users":    {Select: &SelectPermission{}},
					"products": {Select: &SelectPermission{}},
				},
			},
			"anonymous": {
				Tables: map[string]TablePermission{
					"products": {Select: &SelectPermission{}},
				},
			},
		},
	}

	tests := []struct {
		role     string
		table    string
		expected bool
	}{
		{"admin", "users", true},
		{"admin", "products", true},       // Admin has access to all
		{"admin", "nonexistent", true},    // Admin has access to all
		{"user", "users", true},
		{"user", "products", true},
		{"user", "secrets", false},
		{"anonymous", "products", true},
		{"anonymous", "users", false},
	}

	for _, tt := range tests {
		pc := NewPermissionChecker(cfg, tt.role, nil)
		result := pc.CanSelect(tt.table)
		if result != tt.expected {
			t.Errorf("CanSelect(%q) for role %q = %v, want %v",
				tt.table, tt.role, result, tt.expected)
		}
	}
}

func TestPermissionChecker_CanInsert(t *testing.T) {
	cfg := &PermissionsConfig{
		AdminRole:   "admin",
		DefaultRole: "anonymous",
		Roles: map[string]RolePermissions{
			"user": {
				Tables: map[string]TablePermission{
					"posts": {
						Select: &SelectPermission{},
						Insert: &InsertPermission{},
					},
					"users": {
						Select: &SelectPermission{},
						// No insert permission
					},
				},
			},
		},
	}

	tests := []struct {
		role     string
		table    string
		expected bool
	}{
		{"admin", "posts", true},   // Admin can do anything
		{"user", "posts", true},    // User has insert permission
		{"user", "users", false},   // User can't insert into users
		{"user", "other", false},   // User can't insert into unknown table
	}

	for _, tt := range tests {
		pc := NewPermissionChecker(cfg, tt.role, nil)
		result := pc.CanInsert(tt.table)
		if result != tt.expected {
			t.Errorf("CanInsert(%q) for role %q = %v, want %v",
				tt.table, tt.role, result, tt.expected)
		}
	}
}

func TestPermissionChecker_InheritedPermissions(t *testing.T) {
	cfg := &PermissionsConfig{
		Roles: map[string]RolePermissions{
			"base": {
				Tables: map[string]TablePermission{
					"users": {Select: &SelectPermission{}},
				},
			},
			"derived": {
				InheritFrom: "base",
				Tables: map[string]TablePermission{
					"products": {Select: &SelectPermission{}},
				},
			},
		},
	}

	pc := NewPermissionChecker(cfg, "derived", nil)

	// Should have access to products (direct)
	if !pc.CanSelect("products") {
		t.Error("derived role should have select on products")
	}

	// Should have access to users (inherited)
	if !pc.CanSelect("users") {
		t.Error("derived role should have select on users (inherited)")
	}
}

func TestPermissionChecker_BuildFilterSQL(t *testing.T) {
	cfg := &PermissionsConfig{}
	session := SessionVariables{
		"X-User-Id": "123",
	}

	pc := NewPermissionChecker(cfg, "user", session)

	tests := []struct {
		name     string
		filter   *PermissionFilter
		expected string
	}{
		{
			name:     "nil filter",
			filter:   nil,
			expected: "",
		},
		{
			name: "simple equality",
			filter: &PermissionFilter{
				Columns: map[string]map[string]interface{}{
					"user_id": {"_eq": "123"},
				},
			},
			expected: "`user_id` = ?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, _, err := pc.BuildFilterSQL(tt.filter)
			if err != nil {
				t.Fatalf("BuildFilterSQL() error = %v", err)
			}
			if sql != tt.expected {
				t.Errorf("BuildFilterSQL() = %q, want %q", sql, tt.expected)
			}
		})
	}
}

func TestSessionVariables_Get(t *testing.T) {
	sv := SessionVariables{
		"X-User-Id": "123",
		"X-Role":    "user",
	}

	tests := []struct {
		key      string
		expected interface{}
		found    bool
	}{
		{"X-User-Id", "123", true},
		{"X-Role", "user", true},
		{"X-Unknown", nil, false},
	}

	for _, tt := range tests {
		val, found := sv.Get(tt.key)
		if found != tt.found {
			t.Errorf("Get(%q) found = %v, want %v", tt.key, found, tt.found)
		}
		if found && val != tt.expected {
			t.Errorf("Get(%q) = %v, want %v", tt.key, val, tt.expected)
		}
	}
}

func TestSelectPermission(t *testing.T) {
	limit := 100
	perm := SelectPermission{
		Columns:           []string{"id", "name", "email"},
		Limit:             &limit,
		AllowAggregations: true,
		Filter: &PermissionFilter{
			Columns: map[string]map[string]interface{}{
				"is_public": {"_eq": true},
			},
		},
	}

	if len(perm.Columns) != 3 {
		t.Errorf("Columns count = %d, want 3", len(perm.Columns))
	}

	if *perm.Limit != 100 {
		t.Errorf("Limit = %d, want 100", *perm.Limit)
	}

	if !perm.AllowAggregations {
		t.Error("AllowAggregations should be true")
	}

	if perm.Filter == nil {
		t.Error("Filter should not be nil")
	}
}

func TestInsertPermission(t *testing.T) {
	perm := InsertPermission{
		Columns: []string{"name", "email"},
		Set: map[string]interface{}{
			"created_by": "X-User-Id",
		},
	}

	if len(perm.Columns) != 2 {
		t.Errorf("Columns count = %d, want 2", len(perm.Columns))
	}

	if perm.Set["created_by"] != "X-User-Id" {
		t.Errorf("Set[created_by] = %v, want X-User-Id", perm.Set["created_by"])
	}
}

func TestPermissionChecker_MapOperator(t *testing.T) {
	cfg := &PermissionsConfig{}
	pc := NewPermissionChecker(cfg, "user", nil)

	tests := []struct {
		op       string
		expected string
		err      bool
	}{
		{"_eq", "=", false},
		{"_neq", "!=", false},
		{"_gt", ">", false},
		{"_gte", ">=", false},
		{"_lt", "<", false},
		{"_lte", "<=", false},
		{"_in", "IN", false},
		{"_like", "LIKE", false},
		{"_invalid", "", true},
	}

	for _, tt := range tests {
		result, err := pc.mapOperator(tt.op)
		if tt.err {
			if err == nil {
				t.Errorf("mapOperator(%q) expected error, got nil", tt.op)
			}
		} else {
			if err != nil {
				t.Errorf("mapOperator(%q) error = %v", tt.op, err)
			}
			if result != tt.expected {
				t.Errorf("mapOperator(%q) = %q, want %q", tt.op, result, tt.expected)
			}
		}
	}
}
