package connector

import (
	"context"
	"fmt"

	"github.com/hasura/ndc-sdk-go/schema"
	"github.com/your-org/ndc-clickhouse-go/config"
	"github.com/your-org/ndc-clickhouse-go/internal/query"
)

// Query executes a query request
func (c *Connector) Query(ctx context.Context, cfg *config.Configuration, state *State, request *schema.QueryRequest) (schema.QueryResponse, error) {
	// Build SQL from the query request
	sql, args, err := buildQuerySQL(request)
	if err != nil {
		return nil, schema.UnprocessableContentError(fmt.Sprintf("failed to build query: %v", err), nil)
	}

	// Execute the query
	executor := query.NewExecutor(state.Client)
	rows, err := executor.ExecuteQuery(ctx, sql, args)
	if err != nil {
		return nil, schema.UnprocessableContentError(fmt.Sprintf("query execution failed: %v", err), nil)
	}

	// Build the response
	rowSets := []schema.RowSet{
		{
			Rows: convertToRowSetRows(rows),
		},
	}

	return rowSets, nil
}

// buildQuerySQL constructs the SQL query from an NDC query request
func buildQuerySQL(request *schema.QueryRequest) (string, []interface{}, error) {
	builder := query.NewBuilder(request.Collection)

	// Handle fields
	if request.Query.Fields != nil {
		builder.WithFields(request.Query.Fields)
	}

	// Handle predicate (where clause)
	if request.Query.Predicate != nil {
		builder.WithWhere(request.Query.Predicate)
	}

	// Handle order by
	if request.Query.OrderBy != nil {
		builder.WithOrderBy(request.Query.OrderBy)
	}

	// Handle limit
	if request.Query.Limit != nil {
		builder.WithLimit(request.Query.Limit)
	}

	// Handle offset
	if request.Query.Offset != nil {
		builder.WithOffset(request.Query.Offset)
	}

	// Handle aggregates
	if request.Query.Aggregates != nil {
		builder.WithAggregates(request.Query.Aggregates)
	}

	// Handle variables
	if len(request.Variables) > 0 {
		// Process first set of variables (for now)
		if len(request.Variables) > 0 {
			vars := make(map[string]interface{})
			for k, v := range request.Variables[0] {
				vars[k] = v
			}
			builder.WithVariables(vars)
		}
	}

	result, err := builder.Build()
	if err != nil {
		return "", nil, err
	}

	return result.SQL, result.Args, nil
}

// convertToRowSetRows converts query results to RowSet rows format
func convertToRowSetRows(rows []map[string]interface{}) []map[string]schema.RowFieldValue {
	result := make([]map[string]schema.RowFieldValue, len(rows))

	for i, row := range rows {
		resultRow := make(map[string]schema.RowFieldValue)
		for k, v := range row {
			resultRow[k] = v
		}
		result[i] = resultRow
	}

	return result
}

// QueryWithVariables handles queries with multiple variable sets
func (c *Connector) QueryWithVariables(ctx context.Context, cfg *config.Configuration, state *State, request *schema.QueryRequest) (schema.QueryResponse, error) {
	if len(request.Variables) == 0 {
		return c.Query(ctx, cfg, state, request)
	}

	// Execute query for each variable set
	var allRowSets []schema.RowSet

	for _, varSet := range request.Variables {
		// Create a copy of the request with single variable set
		singleRequest := *request
		singleRequest.Variables = []schema.QueryRequestVariablesElem{varSet}

		rowSets, err := c.Query(ctx, cfg, state, &singleRequest)
		if err != nil {
			return nil, err
		}

		allRowSets = append(allRowSets, rowSets...)
	}

	return allRowSets, nil
}
