package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfiguration_Default(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()

	cfg, err := LoadConfiguration(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadConfiguration() returned nil")
	}

	if cfg.Tables == nil {
		t.Error("LoadConfiguration().Tables is nil")
	}

	if cfg.NativeQueries == nil {
		t.Error("LoadConfiguration().NativeQueries is nil")
	}
}

func TestLoadConfiguration_WithFile(t *testing.T) {
	tmpDir := t.TempDir()

	configJSON := `{
		"connection": {
			"url": "clickhouse://localhost:9000",
			"database": "testdb",
			"username": "testuser"
		},
		"tables": {
			"users": {
				"alias": "Users"
			}
		}
	}`

	configPath := filepath.Join(tmpDir, "configuration.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadConfiguration(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}

	if cfg.Connection.URL != "clickhouse://localhost:9000" {
		t.Errorf("Connection.URL = %q, want %q", cfg.Connection.URL, "clickhouse://localhost:9000")
	}

	if cfg.Connection.Database != "testdb" {
		t.Errorf("Connection.Database = %q, want %q", cfg.Connection.Database, "testdb")
	}

	if cfg.Connection.Username != "testuser" {
		t.Errorf("Connection.Username = %q, want %q", cfg.Connection.Username, "testuser")
	}

	tableConfig, ok := cfg.Tables["users"]
	if !ok {
		t.Error("Tables[users] not found")
	}

	if tableConfig.Alias != "Users" {
		t.Errorf("Tables[users].Alias = %q, want %q", tableConfig.Alias, "Users")
	}
}

func TestLoadConfiguration_EnvExpansion(t *testing.T) {
	tmpDir := t.TempDir()

	// Set environment variables
	os.Setenv("TEST_CH_URL", "clickhouse://env-host:9000")
	os.Setenv("TEST_CH_DB", "envdb")
	defer os.Unsetenv("TEST_CH_URL")
	defer os.Unsetenv("TEST_CH_DB")

	configJSON := `{
		"connection": {
			"url": "${TEST_CH_URL}",
			"database": "${TEST_CH_DB}"
		}
	}`

	configPath := filepath.Join(tmpDir, "configuration.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadConfiguration(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfiguration() error = %v", err)
	}

	if cfg.Connection.URL != "clickhouse://env-host:9000" {
		t.Errorf("Connection.URL = %q, want %q", cfg.Connection.URL, "clickhouse://env-host:9000")
	}

	if cfg.Connection.Database != "envdb" {
		t.Errorf("Connection.Database = %q, want %q", cfg.Connection.Database, "envdb")
	}
}

func TestSaveConfiguration(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &Configuration{
		Connection: ConnectionConfig{
			URL:      "clickhouse://localhost:9000",
			Database: "testdb",
		},
		Tables: map[string]TableConfig{
			"users": {Alias: "Users"},
		},
		NativeQueries: make(map[string]NativeQuery),
	}

	err := SaveConfiguration(tmpDir, cfg)
	if err != nil {
		t.Fatalf("SaveConfiguration() error = %v", err)
	}

	// Verify the file was created
	configPath := filepath.Join(tmpDir, "configuration.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Configuration file was not created")
	}

	// Load it back and verify
	loadedCfg, err := LoadConfiguration(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfiguration() after save error = %v", err)
	}

	if loadedCfg.Connection.URL != cfg.Connection.URL {
		t.Errorf("Loaded URL = %q, want %q", loadedCfg.Connection.URL, cfg.Connection.URL)
	}
}

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	tests := []struct {
		input    string
		expected string
	}{
		{"${TEST_VAR}", "test_value"},
		{"$TEST_VAR", "test_value"},
		{"prefix_${TEST_VAR}_suffix", "prefix_test_value_suffix"},
		{"no_vars", "no_vars"},
		{"${UNDEFINED_VAR}", ""},
	}

	for _, tt := range tests {
		result := expandEnv(tt.input)
		if result != tt.expected {
			t.Errorf("expandEnv(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTableConfig(t *testing.T) {
	tableConfig := TableConfig{
		Alias:   "Users",
		Exclude: false,
		Columns: map[string]ColumnConfig{
			"password": {Exclude: true},
			"email":    {Alias: "emailAddress"},
		},
		PrimaryKey: []string{"id"},
	}

	if tableConfig.Alias != "Users" {
		t.Errorf("Alias = %q, want %q", tableConfig.Alias, "Users")
	}

	if tableConfig.Columns["password"].Exclude != true {
		t.Error("password column should be excluded")
	}

	if tableConfig.Columns["email"].Alias != "emailAddress" {
		t.Errorf("email alias = %q, want %q", tableConfig.Columns["email"].Alias, "emailAddress")
	}

	if len(tableConfig.PrimaryKey) != 1 || tableConfig.PrimaryKey[0] != "id" {
		t.Errorf("PrimaryKey = %v, want [id]", tableConfig.PrimaryKey)
	}
}

func TestNativeQuery(t *testing.T) {
	nq := NativeQuery{
		SQL:         "SELECT * FROM users WHERE created_at > {start_date:Date}",
		Description: "Get users created after date",
		Columns: map[string]string{
			"id":    "UUID",
			"name":  "String",
			"email": "String",
		},
		Arguments: map[string]ArgumentConfig{
			"start_date": {
				Type:        "Date",
				Description: "Start date filter",
				Required:    true,
			},
		},
	}

	if nq.SQL == "" {
		t.Error("SQL is empty")
	}

	if len(nq.Columns) != 3 {
		t.Errorf("Columns count = %d, want 3", len(nq.Columns))
	}

	if nq.Arguments["start_date"].Required != true {
		t.Error("start_date should be required")
	}
}
