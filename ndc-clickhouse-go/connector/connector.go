package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/your-org/ndc-clickhouse-go/clickhouse"
	"github.com/your-org/ndc-clickhouse-go/config"
	chschema "github.com/your-org/ndc-clickhouse-go/schema"
	"github.com/your-org/ndc-clickhouse-go/shutdown"
)

// Connector implements the NDC Connector interface for ClickHouse
type Connector struct {
	shutdownManager *shutdown.Manager
}

// State holds the runtime state of the connector
type State struct {
	Client          *clickhouse.Client
	Schema          *schema.SchemaResponse
	TableColumns    map[string][]clickhouse.ColumnInfo
	ShutdownManager *shutdown.Manager
}

// NewConnector creates a new ClickHouse connector instance
func NewConnector() *Connector {
	return &Connector{
		shutdownManager: shutdown.NewManager(shutdown.DefaultConfig()),
	}
}

// NewConnectorWithShutdownConfig creates a new connector with custom shutdown config
func NewConnectorWithShutdownConfig(cfg *shutdown.Config) *Connector {
	return &Connector{
		shutdownManager: shutdown.NewManager(cfg),
	}
}

// ParseConfiguration parses and validates the connector configuration
func (c *Connector) ParseConfiguration(ctx context.Context, configurationDir string) (*config.Configuration, error) {
	cfg, err := config.LoadConfiguration(configurationDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate required fields
	if cfg.Connection.URL == "" && cfg.Connection.Database == "" {
		return nil, fmt.Errorf("either connection URL or database must be specified")
	}

	return cfg, nil
}

// TryInitState initializes the connector state
func (c *Connector) TryInitState(ctx context.Context, cfg *config.Configuration, metrics *connector.TelemetryState) (*State, error) {
	// Create ClickHouse client
	client, err := clickhouse.NewClient(&cfg.Connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create ClickHouse client: %w", err)
	}

	// Register client with shutdown manager for graceful shutdown
	c.shutdownManager.RegisterCloseable("clickhouse-client", client)

	// Register shutdown hook for logging stats
	c.shutdownManager.RegisterHook(shutdown.Hook{
		Name:     "log-client-stats",
		Priority: 10, // Run after connections are drained
		Fn: func(ctx context.Context) error {
			stats := client.GetStats()
			fmt.Printf("ClickHouse client stats at shutdown: queries=%d, successful=%d, failed=%d\n",
				stats.TotalQueries, stats.SuccessfulQueries, stats.FailedQueries)
			return nil
		},
	})

	// Build schema from introspection
	schemaResponse, tableColumns, err := c.buildSchema(ctx, client, cfg)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to build schema: %w", err)
	}

	return &State{
		Client:          client,
		Schema:          schemaResponse,
		TableColumns:    tableColumns,
		ShutdownManager: c.shutdownManager,
	}, nil
}

// Shutdown initiates graceful shutdown of the connector
func (c *Connector) Shutdown(ctx context.Context) error {
	c.shutdownManager.Shutdown(ctx)
	return c.shutdownManager.WaitForShutdown()
}

// ShutdownWithTimeout initiates graceful shutdown with a specific timeout
func (c *Connector) ShutdownWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.Shutdown(ctx)
}

// GetShutdownManager returns the shutdown manager for external access
func (c *Connector) GetShutdownManager() *shutdown.Manager {
	return c.shutdownManager
}

// buildSchema introspects ClickHouse and builds the NDC schema
func (c *Connector) buildSchema(ctx context.Context, client *clickhouse.Client, cfg *config.Configuration) (*schema.SchemaResponse, map[string][]clickhouse.ColumnInfo, error) {
	// Get all tables
	tables, err := client.GetTables(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tables: %w", err)
	}

	// Get all columns
	allColumns, err := client.GetAllColumns(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get columns: %w", err)
	}

	objectTypes := make(schema.SchemaResponseObjectTypes)
	collections := []schema.CollectionInfo{}

	for _, table := range tables {
		// Check if table is excluded
		if tableConfig, ok := cfg.Tables[table.Name]; ok && tableConfig.Exclude {
			continue
		}

		columns, ok := allColumns[table.Name]
		if !ok {
			continue
		}

		// Build object type for this table
		objectType := c.buildObjectType(table.Name, columns, cfg)
		typeName := c.getTypeName(table.Name, cfg)
		objectTypes[typeName] = objectType

		// Build collection info
		collectionName := c.getCollectionName(table.Name, cfg)
		collection := schema.CollectionInfo{
			Name:        collectionName,
			Type:        typeName,
			Arguments:   schema.CollectionInfoArguments{},
			UniquenessConstraints: schema.CollectionInfoUniquenessConstraints{},
			ForeignKeys: schema.CollectionInfoForeignKeys{},
		}

		// Add description if available
		if table.Comment != "" {
			collection.Description = &table.Comment
		}

		collections = append(collections, collection)
	}

	// Add native queries as collections
	for name, nq := range cfg.NativeQueries {
		if nq.ReturnType != "" {
			// Uses existing type
			collection := schema.CollectionInfo{
				Name:      name,
				Type:      nq.ReturnType,
				Arguments: c.buildNativeQueryArguments(nq),
			}
			if nq.Description != "" {
				collection.Description = &nq.Description
			}
			collections = append(collections, collection)
		} else if len(nq.Columns) > 0 {
			// Define inline type
			typeName := name + "_result"
			objectTypes[typeName] = c.buildNativeQueryType(nq)
			collection := schema.CollectionInfo{
				Name:      name,
				Type:      typeName,
				Arguments: c.buildNativeQueryArguments(nq),
			}
			if nq.Description != "" {
				collection.Description = &nq.Description
			}
			collections = append(collections, collection)
		}
	}

	schemaResponse := &schema.SchemaResponse{
		ScalarTypes: chschema.GetScalarTypes(),
		ObjectTypes: objectTypes,
		Collections: collections,
		Functions:   []schema.FunctionInfo{},
		Procedures:  []schema.ProcedureInfo{},
	}

	return schemaResponse, allColumns, nil
}

// buildObjectType builds an NDC object type from ClickHouse columns
func (c *Connector) buildObjectType(tableName string, columns []clickhouse.ColumnInfo, cfg *config.Configuration) schema.ObjectType {
	fields := make(schema.ObjectTypeFields)

	for _, col := range columns {
		// Check if column is excluded
		if tableConfig, ok := cfg.Tables[tableName]; ok {
			if colConfig, ok := tableConfig.Columns[col.Name]; ok && colConfig.Exclude {
				continue
			}
		}

		fieldName := c.getFieldName(tableName, col.Name, cfg)
		fieldType := chschema.ToNDCType(col.Type)

		field := schema.ObjectField{
			Type: fieldType.Encode(),
		}

		if col.Comment != "" {
			field.Description = &col.Comment
		}

		fields[fieldName] = field
	}

	return schema.ObjectType{
		Fields: fields,
	}
}

// buildNativeQueryType builds an object type for a native query
func (c *Connector) buildNativeQueryType(nq config.NativeQuery) schema.ObjectType {
	fields := make(schema.ObjectTypeFields)

	for colName, colType := range nq.Columns {
		fieldType := chschema.ToNDCType(colType)
		fields[colName] = schema.ObjectField{
			Type: fieldType.Encode(),
		}
	}

	return schema.ObjectType{
		Fields: fields,
	}
}

// buildNativeQueryArguments builds arguments for a native query
func (c *Connector) buildNativeQueryArguments(nq config.NativeQuery) schema.CollectionInfoArguments {
	args := make(schema.CollectionInfoArguments)

	for argName, argConfig := range nq.Arguments {
		argType := chschema.ToNDCType(argConfig.Type)
		if !argConfig.Required {
			argType = schema.NewNullableType(argType)
		}

		arg := schema.ArgumentInfo{
			Type: argType.Encode(),
		}
		if argConfig.Description != "" {
			arg.Description = &argConfig.Description
		}

		args[argName] = arg
	}

	return args
}

// getTypeName returns the GraphQL type name for a table
func (c *Connector) getTypeName(tableName string, cfg *config.Configuration) string {
	if tableConfig, ok := cfg.Tables[tableName]; ok && tableConfig.Alias != "" {
		return tableConfig.Alias
	}
	return tableName
}

// getCollectionName returns the collection name for a table
func (c *Connector) getCollectionName(tableName string, cfg *config.Configuration) string {
	if tableConfig, ok := cfg.Tables[tableName]; ok && tableConfig.Alias != "" {
		return tableConfig.Alias
	}
	return tableName
}

// getFieldName returns the GraphQL field name for a column
func (c *Connector) getFieldName(tableName, columnName string, cfg *config.Configuration) string {
	if tableConfig, ok := cfg.Tables[tableName]; ok {
		if colConfig, ok := tableConfig.Columns[columnName]; ok && colConfig.Alias != "" {
			return colConfig.Alias
		}
	}
	return columnName
}

// HealthCheck performs a health check on the connector
func (c *Connector) HealthCheck(ctx context.Context, cfg *config.Configuration, state *State) error {
	if state.Client == nil {
		return fmt.Errorf("client not initialized")
	}

	// Perform a simple query to verify connectivity
	_, err := state.Client.Query(ctx, "SELECT 1")
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// GetCapabilities returns the connector capabilities
func (c *Connector) GetCapabilities(cfg *config.Configuration) *schema.CapabilitiesResponse {
	return &schema.CapabilitiesResponse{
		Version: "0.1.6",
		Capabilities: schema.Capabilities{
			Query: schema.QueryCapabilities{
				Aggregates: &schema.LeafCapability{},
				Variables:  &schema.LeafCapability{},
				NestedFields: schema.NestedFieldCapabilities{
					FilterBy: &schema.LeafCapability{},
					OrderBy:  &schema.LeafCapability{},
				},
			},
			Mutation: schema.MutationCapabilities{},
			Relationships: &schema.RelationshipCapabilities{
				RelationComparisons: &schema.LeafCapability{},
				OrderByAggregate:    &schema.LeafCapability{},
			},
		},
	}
}

// GetSchema returns the connector schema
func (c *Connector) GetSchema(ctx context.Context, cfg *config.Configuration, state *State) (schema.SchemaResponseMarshaler, error) {
	if state.Schema == nil {
		return nil, fmt.Errorf("schema not initialized")
	}
	return state.Schema, nil
}

// QueryExplain returns an explanation of a query
func (c *Connector) QueryExplain(ctx context.Context, cfg *config.Configuration, state *State, request *schema.QueryRequest) (*schema.ExplainResponse, error) {
	// Build the SQL query
	sql, _, err := buildQuerySQL(request)
	if err != nil {
		return nil, err
	}

	return &schema.ExplainResponse{
		Details: schema.ExplainResponseDetails{
			"sql": sql,
		},
	}, nil
}

// MutationExplain returns an explanation of a mutation
func (c *Connector) MutationExplain(ctx context.Context, cfg *config.Configuration, state *State, request *schema.MutationRequest) (*schema.ExplainResponse, error) {
	return &schema.ExplainResponse{
		Details: schema.ExplainResponseDetails{
			"message": "mutations are processed as batch inserts",
		},
	}, nil
}
