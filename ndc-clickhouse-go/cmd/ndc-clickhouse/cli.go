package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
	"github.com/your-org/ndc-clickhouse-go/config"
)

// CLI commands that can be run standalone (without the NDC server)

// IntrospectCmd handles the introspect command
type IntrospectCmd struct {
	URL      string `help:"ClickHouse connection URL" env:"CLICKHOUSE_URL"`
	Database string `help:"Database name" env:"CLICKHOUSE_DATABASE" default:"default"`
	Username string `help:"Username" env:"CLICKHOUSE_USERNAME" default:"default"`
	Password string `help:"Password" env:"CLICKHOUSE_PASSWORD"`
	Output   string `help:"Output directory for configuration" short:"o" default:"./config"`
	Format   string `help:"Output format (json, yaml)" default:"json"`
	AutoRel  bool   `help:"Auto-detect relationships" default:"true"`
}

// Run executes the introspect command
func (cmd *IntrospectCmd) Run() error {
	fmt.Println("🔍 Introspecting ClickHouse database...")

	// Create client
	connConfig := &config.ConnectionConfig{
		URL:      cmd.URL,
		Database: cmd.Database,
		Username: cmd.Username,
		Password: cmd.Password,
	}

	client, err := clickhouse.NewClient(connConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get tables
	fmt.Println("📋 Fetching tables...")
	tables, err := client.GetTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tables: %w", err)
	}
	fmt.Printf("   Found %d tables\n", len(tables))

	// Get columns for all tables
	fmt.Println("📊 Fetching columns...")
	allColumns, err := client.GetAllColumns(ctx)
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	// Build configuration
	cfg := &config.Configuration{
		Connection: config.ConnectionConfig{
			URL:          "${CLICKHOUSE_URL}",
			Database:     "${CLICKHOUSE_DATABASE}",
			Username:     "${CLICKHOUSE_USERNAME}",
			Password:     "${CLICKHOUSE_PASSWORD}",
			MaxOpenConns: 10,
			MaxIdleConns: 5,
		},
		Tables:        make(map[string]config.TableConfig),
		NativeQueries: make(map[string]config.NativeQuery),
		Metadata: config.MetadataConfig{
			Version:     "1.0.0",
			Description: fmt.Sprintf("Auto-generated configuration for %s", cmd.Database),
		},
	}

	// Build table configurations
	for _, table := range tables {
		columns := allColumns[table.Name]
		tableConfig := config.TableConfig{
			Columns: make(map[string]config.ColumnConfig),
		}

		// Find primary keys
		var primaryKeys []string
		for _, col := range columns {
			if col.IsPrimaryKey {
				primaryKeys = append(primaryKeys, col.Name)
			}
		}
		tableConfig.PrimaryKey = primaryKeys

		cfg.Tables[table.Name] = tableConfig
	}

	// Auto-detect relationships
	if cmd.AutoRel {
		fmt.Println("🔗 Detecting relationships...")
		relationships := detectRelationships(tables, allColumns)
		cfg.Relationships = config.RelationshipsConfig{
			Relationships: relationships,
			AutoDetect:    true,
		}
		fmt.Printf("   Found %d relationships\n", len(relationships))
	}

	// Generate default permissions
	fmt.Println("🔒 Generating default permissions...")
	cfg.Permissions = generateDefaultPermissions(tables)

	// Save configuration
	fmt.Printf("💾 Saving configuration to %s...\n", cmd.Output)
	if err := os.MkdirAll(cmd.Output, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	configPath := filepath.Join(cmd.Output, "configuration.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write configuration: %w", err)
	}

	// Generate schema documentation
	schemaDocPath := filepath.Join(cmd.Output, "schema.md")
	schemaDoc := generateSchemaDocumentation(tables, allColumns)
	if err := os.WriteFile(schemaDocPath, []byte(schemaDoc), 0644); err != nil {
		return fmt.Errorf("failed to write schema documentation: %w", err)
	}

	fmt.Println("✅ Introspection complete!")
	fmt.Printf("   Configuration: %s\n", configPath)
	fmt.Printf("   Schema docs:   %s\n", schemaDocPath)

	return nil
}

// detectRelationships auto-detects relationships based on naming conventions
func detectRelationships(tables []clickhouse.TableInfo, allColumns map[string][]clickhouse.ColumnInfo) []config.Relationship {
	var relationships []config.Relationship
	tableNames := make(map[string]bool)

	for _, t := range tables {
		tableNames[t.Name] = true
	}

	for _, table := range tables {
		columns := allColumns[table.Name]

		for _, col := range columns {
			// Check for foreign key pattern: {table}_id or {table}Id
			for targetTable := range tableNames {
				if targetTable == table.Name {
					continue
				}

				// Check common FK patterns
				fkPatterns := []string{
					targetTable + "_id",
					targetTable + "Id",
					targetTable + "_uuid",
					singularize(targetTable) + "_id",
					singularize(targetTable) + "Id",
				}

				for _, pattern := range fkPatterns {
					if strings.EqualFold(col.Name, pattern) {
						// Found a potential foreign key
						// Object relationship (many-to-one)
						relationships = append(relationships, config.Relationship{
							Name:        singularize(targetTable),
							Type:        config.ObjectRelationship,
							SourceTable: table.Name,
							TargetTable: targetTable,
							ColumnMapping: map[string]string{
								col.Name: "id",
							},
						})

						// Array relationship (one-to-many) on the target table
						relationships = append(relationships, config.Relationship{
							Name:        pluralize(table.Name),
							Type:        config.ArrayRelationship,
							SourceTable: targetTable,
							TargetTable: table.Name,
							ColumnMapping: map[string]string{
								"id": col.Name,
							},
						})
						break
					}
				}
			}
		}
	}

	return relationships
}

// generateDefaultPermissions creates default permission structure
func generateDefaultPermissions(tables []clickhouse.TableInfo) config.PermissionsConfig {
	adminTables := make(map[string]config.TablePermission)
	userTables := make(map[string]config.TablePermission)
	anonymousTables := make(map[string]config.TablePermission)

	for _, table := range tables {
		// Admin has full access
		adminTables[table.Name] = config.TablePermission{
			Select: &config.SelectPermission{
				AllowAggregations: true,
			},
			Insert: &config.InsertPermission{},
		}

		// User has select access
		userTables[table.Name] = config.TablePermission{
			Select: &config.SelectPermission{
				AllowAggregations: true,
			},
		}

		// Anonymous has limited select access
		limit := 100
		anonymousTables[table.Name] = config.TablePermission{
			Select: &config.SelectPermission{
				Limit:             &limit,
				AllowAggregations: false,
			},
		}
	}

	return config.PermissionsConfig{
		DefaultRole: "anonymous",
		AdminRole:   "admin",
		Roles: map[string]config.RolePermissions{
			"admin": {
				Tables: adminTables,
			},
			"user": {
				Tables: userTables,
			},
			"anonymous": {
				Tables: anonymousTables,
			},
		},
	}
}

// generateSchemaDocumentation creates markdown documentation for the schema
func generateSchemaDocumentation(tables []clickhouse.TableInfo, allColumns map[string][]clickhouse.ColumnInfo) string {
	var sb strings.Builder

	sb.WriteString("# Database Schema Documentation\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format(time.RFC3339)))
	sb.WriteString("## Tables\n\n")

	for _, table := range tables {
		sb.WriteString(fmt.Sprintf("### %s\n\n", table.Name))

		if table.Comment != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", table.Comment))
		}

		sb.WriteString(fmt.Sprintf("- **Engine**: %s\n", table.Engine))
		sb.WriteString(fmt.Sprintf("- **Rows**: %d\n", table.TotalRows))
		sb.WriteString(fmt.Sprintf("- **Size**: %s\n\n", formatBytes(table.TotalBytes)))

		columns := allColumns[table.Name]
		if len(columns) > 0 {
			sb.WriteString("| Column | Type | Primary Key | Description |\n")
			sb.WriteString("|--------|------|-------------|-------------|\n")

			for _, col := range columns {
				pk := ""
				if col.IsPrimaryKey {
					pk = "✓"
				}
				desc := col.Comment
				if desc == "" {
					desc = "-"
				}
				sb.WriteString(fmt.Sprintf("| %s | `%s` | %s | %s |\n",
					col.Name, col.Type, pk, desc))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

// Helper functions

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "es") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

func pluralize(s string) string {
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "sh") {
		return s + "es"
	}
	return s + "s"
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ValidateCmd validates the configuration
type ValidateCmd struct {
	Config string `help:"Configuration directory" short:"c" default:"./config"`
}

// Run executes the validate command
func (cmd *ValidateCmd) Run() error {
	fmt.Println("🔍 Validating configuration...")

	cfg, err := config.LoadConfiguration(cmd.Config)
	if err != nil {
		return fmt.Errorf("❌ Failed to load configuration: %w", err)
	}

	var errors []string
	var warnings []string

	// Validate connection
	if cfg.Connection.URL == "" && cfg.Connection.Database == "" {
		errors = append(errors, "Connection URL or database must be specified")
	}

	// Validate tables
	for name, table := range cfg.Tables {
		if table.Alias != "" && !isValidIdentifier(table.Alias) {
			errors = append(errors, fmt.Sprintf("Invalid alias for table %s: %s", name, table.Alias))
		}
	}

	// Validate relationships
	for _, rel := range cfg.Relationships.Relationships {
		if rel.SourceTable == "" {
			errors = append(errors, fmt.Sprintf("Relationship %s: source_table is required", rel.Name))
		}
		if rel.TargetTable == "" {
			errors = append(errors, fmt.Sprintf("Relationship %s: target_table is required", rel.Name))
		}
		if len(rel.ColumnMapping) == 0 {
			errors = append(errors, fmt.Sprintf("Relationship %s: column_mapping is required", rel.Name))
		}
	}

	// Validate permissions
	for roleName, role := range cfg.Permissions.Roles {
		for tableName := range role.Tables {
			if _, ok := cfg.Tables[tableName]; !ok {
				warnings = append(warnings, fmt.Sprintf("Role %s has permission for unknown table: %s", roleName, tableName))
			}
		}
	}

	// Print results
	if len(errors) > 0 {
		fmt.Println("\n❌ Errors:")
		for _, e := range errors {
			fmt.Printf("   - %s\n", e)
		}
	}

	if len(warnings) > 0 {
		fmt.Println("\n⚠️  Warnings:")
		for _, w := range warnings {
			fmt.Printf("   - %s\n", w)
		}
	}

	if len(errors) == 0 {
		fmt.Println("\n✅ Configuration is valid!")
		return nil
	}

	return fmt.Errorf("configuration has %d errors", len(errors))
}

func isValidIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
				(c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// PrintSchemaCmd prints the GraphQL schema
type PrintSchemaCmd struct {
	Config string `help:"Configuration directory" short:"c" default:"./config"`
	Format string `help:"Output format (graphql, json)" default:"graphql"`
}

// Run executes the print-schema command
func (cmd *PrintSchemaCmd) Run() error {
	fmt.Println("📋 Loading schema...")

	cfg, err := config.LoadConfiguration(cmd.Config)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Connect to ClickHouse
	client, err := clickhouse.NewClient(&cfg.Connection)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()

	ctx := context.Background()
	tables, _ := client.GetTables(ctx)
	allColumns, _ := client.GetAllColumns(ctx)

	// Print schema
	fmt.Println("\n# GraphQL Schema\n")

	for _, table := range tables {
		if tableConfig, ok := cfg.Tables[table.Name]; ok && tableConfig.Exclude {
			continue
		}

		typeName := table.Name
		if tableConfig, ok := cfg.Tables[table.Name]; ok && tableConfig.Alias != "" {
			typeName = tableConfig.Alias
		}

		fmt.Printf("type %s {\n", typeName)

		columns := allColumns[table.Name]
		for _, col := range columns {
			gqlType := clickHouseToGraphQL(col.Type)
			fmt.Printf("  %s: %s\n", col.Name, gqlType)
		}

		// Print relationships
		for _, rel := range cfg.Relationships.Relationships {
			if rel.SourceTable == table.Name {
				if rel.Type == config.ObjectRelationship {
					fmt.Printf("  %s: %s\n", rel.Name, rel.TargetTable)
				} else {
					fmt.Printf("  %s: [%s!]!\n", rel.Name, rel.TargetTable)
				}
			}
		}

		fmt.Println("}\n")
	}

	return nil
}

func clickHouseToGraphQL(chType string) string {
	switch {
	case strings.HasPrefix(chType, "Nullable"):
		inner := chType[9 : len(chType)-1]
		return clickHouseToGraphQL(inner)
	case strings.HasPrefix(chType, "Array"):
		inner := chType[6 : len(chType)-1]
		return "[" + clickHouseToGraphQL(inner) + "]!"
	case strings.HasPrefix(chType, "Int"), strings.HasPrefix(chType, "UInt"):
		return "Int!"
	case strings.HasPrefix(chType, "Float"), strings.HasPrefix(chType, "Decimal"):
		return "Float!"
	case chType == "String", strings.HasPrefix(chType, "FixedString"):
		return "String!"
	case chType == "Bool":
		return "Boolean!"
	case chType == "UUID":
		return "UUID!"
	case strings.HasPrefix(chType, "Date"):
		return "DateTime!"
	default:
		return "String!"
	}
}
