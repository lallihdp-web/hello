package query

import (
	"context"
	"fmt"
	"reflect"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
)

// Executor executes ClickHouse queries and maps results
type Executor struct {
	client *clickhouse.Client
}

// NewExecutor creates a new query executor
func NewExecutor(client *clickhouse.Client) *Executor {
	return &Executor{client: client}
}

// ExecuteQuery executes a query and returns the results as a slice of maps
func (e *Executor) ExecuteQuery(ctx context.Context, sql string, args []interface{}) ([]map[string]interface{}, error) {
	rows, err := e.client.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column information
	columnTypes := rows.ColumnTypes()
	columnNames := make([]string, len(columnTypes))
	for i, ct := range columnTypes {
		columnNames[i] = ct.Name()
	}

	var results []map[string]interface{}

	for rows.Next() {
		// Create a slice of interface{} to scan into
		values := make([]interface{}, len(columnNames))
		valuePtrs := make([]interface{}, len(columnNames))

		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}

		// Build the result row
		row := make(map[string]interface{})
		for i, name := range columnNames {
			row[name] = convertValue(values[i])
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return results, nil
}

// ExecuteAggregateQuery executes an aggregate query
func (e *Executor) ExecuteAggregateQuery(ctx context.Context, sql string, args []interface{}) (map[string]interface{}, error) {
	rows, err := e.ExecuteQuery(ctx, sql, args)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return make(map[string]interface{}), nil
	}

	return rows[0], nil
}

// convertValue converts ClickHouse values to JSON-serializable Go values
func convertValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	// Handle reflect.Value
	if rv, ok := v.(reflect.Value); ok {
		if !rv.IsValid() {
			return nil
		}
		v = rv.Interface()
	}

	switch val := v.(type) {
	case *interface{}:
		if val == nil {
			return nil
		}
		return convertValue(*val)

	// Numeric types
	case int8:
		return int64(val)
	case int16:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case uint8:
		return int64(val)
	case uint16:
		return int64(val)
	case uint32:
		return int64(val)
	case uint64:
		// Handle potential overflow for very large values
		if val > uint64(1<<63-1) {
			return fmt.Sprintf("%d", val)
		}
		return int64(val)
	case float32:
		return float64(val)
	case float64:
		return val

	// String and bytes
	case string:
		return val
	case []byte:
		return string(val)

	// Boolean
	case bool:
		return val

	// Arrays
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = convertValue(item)
		}
		return result

	// Maps
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, item := range val {
			result[k] = convertValue(item)
		}
		return result

	default:
		// Use reflection for other types
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			result := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result[i] = convertValue(rv.Index(i).Interface())
			}
			return result
		case reflect.Map:
			result := make(map[string]interface{})
			for _, key := range rv.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				result[keyStr] = convertValue(rv.MapIndex(key).Interface())
			}
			return result
		case reflect.Ptr:
			if rv.IsNil() {
				return nil
			}
			return convertValue(rv.Elem().Interface())
		case reflect.Struct:
			// Handle time.Time and other structs
			return fmt.Sprintf("%v", v)
		default:
			return v
		}
	}
}
