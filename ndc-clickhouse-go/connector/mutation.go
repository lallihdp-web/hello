package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/your-org/ndc-clickhouse-go/config"
	"github.com/your-org/ndc-clickhouse-go/internal/query"
)

// Mutation executes a mutation request
func (c *Connector) Mutation(ctx context.Context, cfg *config.Configuration, state *State, request *schema.MutationRequest) (*schema.MutationResponse, error) {
	results := make([]schema.MutationOperationResults, len(request.Operations))

	for i, op := range request.Operations {
		result, err := c.executeMutationOperation(ctx, cfg, state, op)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}

	return &schema.MutationResponse{
		OperationResults: results,
	}, nil
}

// executeMutationOperation executes a single mutation operation
func (c *Connector) executeMutationOperation(ctx context.Context, cfg *config.Configuration, state *State, op schema.MutationOperation) (schema.MutationOperationResults, error) {
	switch op.Type {
	case schema.MutationOperationTypeProcedure:
		return c.executeProcedure(ctx, cfg, state, op)
	default:
		return nil, schema.UnprocessableContentError(fmt.Sprintf("unsupported mutation type: %s", op.Type), nil)
	}
}

// executeProcedure executes a procedure (INSERT, UPDATE, DELETE)
func (c *Connector) executeProcedure(ctx context.Context, cfg *config.Configuration, state *State, op schema.MutationOperation) (schema.MutationOperationResults, error) {
	// Check if this is an insert operation
	if strings.HasPrefix(op.Name, "insert_") {
		return c.executeInsert(ctx, cfg, state, op)
	}

	return nil, schema.UnprocessableContentError(fmt.Sprintf("unsupported procedure: %s", op.Name), nil)
}

// executeInsert executes an INSERT operation
func (c *Connector) executeInsert(ctx context.Context, cfg *config.Configuration, state *State, op schema.MutationOperation) (schema.MutationOperationResults, error) {
	// Extract table name from procedure name (insert_<table>)
	tableName := strings.TrimPrefix(op.Name, "insert_")

	// Get the objects to insert from arguments
	objectsArg, ok := op.Arguments["objects"]
	if !ok {
		return nil, schema.UnprocessableContentError("missing 'objects' argument for insert", nil)
	}

	objects, ok := objectsArg.([]interface{})
	if !ok {
		return nil, schema.UnprocessableContentError("'objects' must be an array", nil)
	}

	if len(objects) == 0 {
		return schema.NewProcedureResult(map[string]interface{}{
			"affected_rows": 0,
		}).Encode(), nil
	}

	// Build and execute batch insert
	affectedRows, err := c.batchInsert(ctx, state, tableName, objects)
	if err != nil {
		return nil, schema.UnprocessableContentError(fmt.Sprintf("insert failed: %v", err), nil)
	}

	return schema.NewProcedureResult(map[string]interface{}{
		"affected_rows": affectedRows,
	}).Encode(), nil
}

// batchInsert performs a batch insert into a ClickHouse table
func (c *Connector) batchInsert(ctx context.Context, state *State, tableName string, objects []interface{}) (int, error) {
	if len(objects) == 0 {
		return 0, nil
	}

	// Get column info for the table
	columns, ok := state.TableColumns[tableName]
	if !ok {
		return 0, fmt.Errorf("table not found: %s", tableName)
	}

	// Get the first object to determine which columns to insert
	firstObj, ok := objects[0].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid object format")
	}

	// Build column list from first object keys
	var insertColumns []string
	var columnIndices []int

	for i, col := range columns {
		if _, exists := firstObj[col.Name]; exists {
			insertColumns = append(insertColumns, col.Name)
			columnIndices = append(columnIndices, i)
		}
	}

	if len(insertColumns) == 0 {
		return 0, fmt.Errorf("no columns to insert")
	}

	// Build INSERT statement
	quotedCols := make([]string, len(insertColumns))
	for i, col := range insertColumns {
		quotedCols[i] = query.QuoteIdentifier(col)
	}

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s)",
		query.QuoteIdentifier(tableName),
		strings.Join(quotedCols, ", "),
	)

	// Prepare batch
	batch, err := state.Client.PrepareBatch(ctx, sql)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare batch: %w", err)
	}

	// Add rows to batch
	for _, obj := range objects {
		objMap, ok := obj.(map[string]interface{})
		if !ok {
			continue
		}

		// Build row values
		values := make([]interface{}, len(insertColumns))
		for i, col := range insertColumns {
			values[i] = objMap[col]
		}

		if err := batch.Append(values...); err != nil {
			return 0, fmt.Errorf("failed to append row: %w", err)
		}
	}

	// Execute batch
	if err := batch.Send(); err != nil {
		return 0, fmt.Errorf("failed to send batch: %w", err)
	}

	return len(objects), nil
}

// GenerateInsertProcedures generates insert procedures for all tables
func GenerateInsertProcedures(tables []string, objectTypes schema.SchemaResponseObjectTypes) []schema.ProcedureInfo {
	var procedures []schema.ProcedureInfo

	for _, table := range tables {
		objectType, ok := objectTypes[table]
		if !ok {
			continue
		}

		// Create input type name
		inputTypeName := table + "_insert_input"

		// Build arguments
		args := schema.ProcedureInfoArguments{
			"objects": schema.ArgumentInfo{
				Type: schema.NewArrayType(schema.NewNamedType(inputTypeName)).Encode(),
			},
		}

		// Build result type
		resultType := schema.NewNamedType(table + "_mutation_response")

		procedure := schema.ProcedureInfo{
			Name:       "insert_" + table,
			Arguments:  args,
			ResultType: resultType.Encode(),
		}

		desc := fmt.Sprintf("Insert rows into %s", table)
		procedure.Description = &desc

		procedures = append(procedures, procedure)

		// Note: In a full implementation, you'd also add the input types
		// and mutation response types to objectTypes
		_ = objectType
	}

	return procedures
}

// MutationResponseType returns the mutation response object type
func MutationResponseType() schema.ObjectType {
	return schema.ObjectType{
		Fields: schema.ObjectTypeFields{
			"affected_rows": schema.ObjectField{
				Type: schema.NewNamedType("Int64").Encode(),
			},
		},
	}
}
