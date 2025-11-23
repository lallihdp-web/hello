package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
)

// Executor executes GraphQL queries against ClickHouse
type Executor struct {
	client *clickhouse.Client
	schema *Schema
}

// NewExecutor creates a new executor
func NewExecutor(client *clickhouse.Client, schema *Schema) *Executor {
	return &Executor{
		client: client,
		schema: schema,
	}
}

// Execute executes a GraphQL request
func (e *Executor) Execute(ctx context.Context, req GraphQLRequest) (*GraphQLResponse, error) {
	// Parse the query
	parsed, err := e.parseQuery(req.Query)
	if err != nil {
		return &GraphQLResponse{
			Errors: []GraphQLError{{Message: fmt.Sprintf("Parse error: %v", err)}},
		}, nil
	}

	// Execute each query field
	data := make(map[string]interface{})

	for _, field := range parsed.Fields {
		result, err := e.executeField(ctx, field, req.Variables)
		if err != nil {
			return &GraphQLResponse{
				Errors: []GraphQLError{{Message: err.Error(), Path: []interface{}{field.Name}}},
			}, nil
		}
		data[field.Alias] = result
	}

	return &GraphQLResponse{Data: data}, nil
}

// ParsedQuery represents a parsed GraphQL query
type ParsedQuery struct {
	Operation string
	Name      string
	Fields    []ParsedField
}

// ParsedField represents a parsed field
type ParsedField struct {
	Name       string
	Alias      string
	Arguments  map[string]interface{}
	Selections []ParsedField
}

// parseQuery parses a GraphQL query string (simplified parser)
func (e *Executor) parseQuery(query string) (*ParsedQuery, error) {
	// Remove comments
	re := regexp.MustCompile(`#[^\n]*`)
	query = re.ReplaceAllString(query, "")

	// Trim whitespace
	query = strings.TrimSpace(query)

	parsed := &ParsedQuery{
		Operation: "query",
		Fields:    []ParsedField{},
	}

	// Check for operation type
	if strings.HasPrefix(query, "query") {
		query = strings.TrimPrefix(query, "query")
		query = strings.TrimSpace(query)
	} else if strings.HasPrefix(query, "mutation") {
		parsed.Operation = "mutation"
		query = strings.TrimPrefix(query, "mutation")
		query = strings.TrimSpace(query)
	}

	// Extract operation name if present
	if !strings.HasPrefix(query, "{") {
		nameEnd := strings.IndexAny(query, "({")
		if nameEnd > 0 {
			parsed.Name = strings.TrimSpace(query[:nameEnd])
			query = query[nameEnd:]
		}
	}

	// Skip variables if present
	if strings.HasPrefix(query, "(") {
		depth := 0
		for i, c := range query {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					query = strings.TrimSpace(query[i+1:])
					break
				}
			}
		}
	}

	// Parse selection set
	if !strings.HasPrefix(query, "{") {
		return nil, fmt.Errorf("expected '{' at start of selection set")
	}

	fields, err := e.parseSelectionSet(query[1:])
	if err != nil {
		return nil, err
	}

	parsed.Fields = fields
	return parsed, nil
}

// parseSelectionSet parses fields within curly braces
func (e *Executor) parseSelectionSet(content string) ([]ParsedField, error) {
	var fields []ParsedField
	content = strings.TrimSpace(content)

	for len(content) > 0 && content[0] != '}' {
		field, remaining, err := e.parseField(content)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
		content = strings.TrimSpace(remaining)
	}

	return fields, nil
}

// parseField parses a single field
func (e *Executor) parseField(content string) (ParsedField, string, error) {
	content = strings.TrimSpace(content)

	field := ParsedField{
		Arguments:  make(map[string]interface{}),
		Selections: []ParsedField{},
	}

	// Parse field name (and alias if present)
	nameEnd := strings.IndexAny(content, "({: \n\t}")
	if nameEnd < 0 {
		nameEnd = len(content)
	}

	name := strings.TrimSpace(content[:nameEnd])
	content = strings.TrimSpace(content[nameEnd:])

	// Check for alias
	if strings.HasPrefix(content, ":") {
		field.Alias = name
		content = strings.TrimSpace(content[1:])

		// Get actual field name
		nameEnd = strings.IndexAny(content, "({ \n\t}")
		if nameEnd < 0 {
			nameEnd = len(content)
		}
		name = strings.TrimSpace(content[:nameEnd])
		content = strings.TrimSpace(content[nameEnd:])
	} else {
		field.Alias = name
	}

	field.Name = name

	// Parse arguments if present
	if strings.HasPrefix(content, "(") {
		args, remaining, err := e.parseArguments(content[1:])
		if err != nil {
			return field, "", err
		}
		field.Arguments = args
		content = strings.TrimSpace(remaining)
	}

	// Parse nested selection set if present
	if strings.HasPrefix(content, "{") {
		// Find matching closing brace
		depth := 1
		end := 1
		for end < len(content) && depth > 0 {
			if content[end] == '{' {
				depth++
			} else if content[end] == '}' {
				depth--
			}
			end++
		}

		nested, err := e.parseSelectionSet(content[1:end])
		if err != nil {
			return field, "", err
		}
		field.Selections = nested
		content = strings.TrimSpace(content[end:])
	}

	return field, content, nil
}

// parseArguments parses function arguments
func (e *Executor) parseArguments(content string) (map[string]interface{}, string, error) {
	args := make(map[string]interface{})

	for len(content) > 0 && content[0] != ')' {
		content = strings.TrimSpace(content)

		// Parse argument name
		colonIdx := strings.Index(content, ":")
		if colonIdx < 0 {
			return nil, "", fmt.Errorf("expected ':' in argument")
		}

		argName := strings.TrimSpace(content[:colonIdx])
		content = strings.TrimSpace(content[colonIdx+1:])

		// Parse argument value
		value, remaining, err := e.parseValue(content)
		if err != nil {
			return nil, "", err
		}

		args[argName] = value
		content = strings.TrimSpace(remaining)

		// Skip comma if present
		if len(content) > 0 && content[0] == ',' {
			content = strings.TrimSpace(content[1:])
		}
	}

	if len(content) > 0 && content[0] == ')' {
		content = content[1:]
	}

	return args, content, nil
}

// parseValue parses a GraphQL value
func (e *Executor) parseValue(content string) (interface{}, string, error) {
	content = strings.TrimSpace(content)

	if len(content) == 0 {
		return nil, "", fmt.Errorf("unexpected end of input")
	}

	switch content[0] {
	case '"':
		// String value
		end := 1
		for end < len(content) && content[end] != '"' {
			if content[end] == '\\' {
				end++
			}
			end++
		}
		if end >= len(content) {
			return nil, "", fmt.Errorf("unterminated string")
		}
		return content[1:end], content[end+1:], nil

	case '[':
		// Array value
		var arr []interface{}
		content = strings.TrimSpace(content[1:])
		for len(content) > 0 && content[0] != ']' {
			val, remaining, err := e.parseValue(content)
			if err != nil {
				return nil, "", err
			}
			arr = append(arr, val)
			content = strings.TrimSpace(remaining)
			if len(content) > 0 && content[0] == ',' {
				content = strings.TrimSpace(content[1:])
			}
		}
		if len(content) > 0 {
			content = content[1:]
		}
		return arr, content, nil

	case '{':
		// Object value
		obj := make(map[string]interface{})
		content = strings.TrimSpace(content[1:])
		for len(content) > 0 && content[0] != '}' {
			colonIdx := strings.Index(content, ":")
			if colonIdx < 0 {
				return nil, "", fmt.Errorf("expected ':' in object")
			}
			key := strings.TrimSpace(content[:colonIdx])
			content = strings.TrimSpace(content[colonIdx+1:])

			val, remaining, err := e.parseValue(content)
			if err != nil {
				return nil, "", err
			}
			obj[key] = val
			content = strings.TrimSpace(remaining)
			if len(content) > 0 && content[0] == ',' {
				content = strings.TrimSpace(content[1:])
			}
		}
		if len(content) > 0 {
			content = content[1:]
		}
		return obj, content, nil

	default:
		// Number, boolean, enum, or null
		end := strings.IndexAny(content, ",)] \n\t}")
		if end < 0 {
			end = len(content)
		}
		token := content[:end]
		remaining := content[end:]

		switch token {
		case "true":
			return true, remaining, nil
		case "false":
			return false, remaining, nil
		case "null":
			return nil, remaining, nil
		default:
			// Try number
			if i, err := strconv.ParseInt(token, 10, 64); err == nil {
				return i, remaining, nil
			}
			if f, err := strconv.ParseFloat(token, 64); err == nil {
				return f, remaining, nil
			}
			// Enum value
			return token, remaining, nil
		}
	}
}

// executeField executes a single query field
func (e *Executor) executeField(ctx context.Context, field ParsedField, variables map[string]interface{}) (interface{}, error) {
	// Check for introspection
	if field.Name == "__schema" {
		return e.executeIntrospection(field)
	}

	// Check if it's an aggregate query
	if strings.HasSuffix(field.Name, "_aggregate") {
		return e.executeAggregate(ctx, field, variables)
	}

	// Get query field definition
	qf := e.schema.GetQueryField(field.Name)
	if qf == nil {
		return nil, fmt.Errorf("unknown field: %s", field.Name)
	}

	objType := e.schema.GetType(qf.Type)
	if objType == nil {
		return nil, fmt.Errorf("unknown type: %s", qf.Type)
	}

	// Build SQL query
	sql, params := e.buildSQL(qf.TableName, objType, field, variables)

	// Execute query
	rows, err := e.client.Query(ctx, sql, params...)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	// Get column info
	columnTypes := rows.ColumnTypes()
	columns := make([]string, len(columnTypes))
	for i, ct := range columnTypes {
		columns[i] = ct.Name()
	}

	// Read results
	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, nil
}

// buildSQL builds a SQL query from GraphQL field
func (e *Executor) buildSQL(tableName string, objType *ObjectType, field ParsedField, variables map[string]interface{}) (string, []interface{}) {
	var params []interface{}

	// Build SELECT columns
	selectCols := []string{}
	for _, sel := range field.Selections {
		if f, ok := objType.Fields[sel.Name]; ok {
			selectCols = append(selectCols, f.ColumnName)
		}
	}

	if len(selectCols) == 0 {
		// Select all if no specific fields requested
		for _, f := range objType.Fields {
			selectCols = append(selectCols, f.ColumnName)
		}
	}

	sql := fmt.Sprintf("SELECT %s FROM %s", strings.Join(selectCols, ", "), tableName)

	// Build WHERE clause
	if where, ok := field.Arguments["where"]; ok {
		if whereMap, ok := where.(map[string]interface{}); ok {
			whereClause, whereParams := e.buildWhereClause(objType, whereMap)
			if whereClause != "" {
				sql += " WHERE " + whereClause
				params = append(params, whereParams...)
			}
		}
	}

	// Build ORDER BY clause
	if orderBy, ok := field.Arguments["order_by"]; ok {
		orderClause := e.buildOrderByClause(objType, orderBy)
		if orderClause != "" {
			sql += " ORDER BY " + orderClause
		}
	}

	// Add LIMIT
	limit := 100
	if l, ok := field.Arguments["limit"]; ok {
		switch v := l.(type) {
		case int64:
			limit = int(v)
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	sql += fmt.Sprintf(" LIMIT %d", limit)

	// Add OFFSET
	offset := 0
	if o, ok := field.Arguments["offset"]; ok {
		switch v := o.(type) {
		case int64:
			offset = int(v)
		case float64:
			offset = int(v)
		case int:
			offset = v
		}
	}
	if offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", offset)
	}

	return sql, params
}

// buildWhereClause builds a WHERE clause from GraphQL filter
func (e *Executor) buildWhereClause(objType *ObjectType, where map[string]interface{}) (string, []interface{}) {
	var conditions []string
	var params []interface{}

	for key, value := range where {
		switch key {
		case "_and":
			if arr, ok := value.([]interface{}); ok {
				var andConds []string
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						cond, p := e.buildWhereClause(objType, m)
						if cond != "" {
							andConds = append(andConds, "("+cond+")")
							params = append(params, p...)
						}
					}
				}
				if len(andConds) > 0 {
					conditions = append(conditions, "("+strings.Join(andConds, " AND ")+")")
				}
			}

		case "_or":
			if arr, ok := value.([]interface{}); ok {
				var orConds []string
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						cond, p := e.buildWhereClause(objType, m)
						if cond != "" {
							orConds = append(orConds, "("+cond+")")
							params = append(params, p...)
						}
					}
				}
				if len(orConds) > 0 {
					conditions = append(conditions, "("+strings.Join(orConds, " OR ")+")")
				}
			}

		case "_not":
			if m, ok := value.(map[string]interface{}); ok {
				cond, p := e.buildWhereClause(objType, m)
				if cond != "" {
					conditions = append(conditions, "NOT ("+cond+")")
					params = append(params, p...)
				}
			}

		default:
			// Field comparison
			if f, ok := objType.Fields[key]; ok {
				if compMap, ok := value.(map[string]interface{}); ok {
					cond, p := e.buildFieldComparison(f.ColumnName, compMap)
					if cond != "" {
						conditions = append(conditions, cond)
						params = append(params, p...)
					}
				}
			}
		}
	}

	return strings.Join(conditions, " AND "), params
}

// buildFieldComparison builds comparison for a single field
func (e *Executor) buildFieldComparison(column string, comp map[string]interface{}) (string, []interface{}) {
	var conditions []string
	var params []interface{}

	for op, value := range comp {
		switch op {
		case "_eq":
			conditions = append(conditions, fmt.Sprintf("%s = ?", column))
			params = append(params, value)
		case "_neq":
			conditions = append(conditions, fmt.Sprintf("%s != ?", column))
			params = append(params, value)
		case "_gt":
			conditions = append(conditions, fmt.Sprintf("%s > ?", column))
			params = append(params, value)
		case "_gte":
			conditions = append(conditions, fmt.Sprintf("%s >= ?", column))
			params = append(params, value)
		case "_lt":
			conditions = append(conditions, fmt.Sprintf("%s < ?", column))
			params = append(params, value)
		case "_lte":
			conditions = append(conditions, fmt.Sprintf("%s <= ?", column))
			params = append(params, value)
		case "_in":
			if arr, ok := value.([]interface{}); ok {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					placeholders[i] = "?"
					params = append(params, v)
				}
				conditions = append(conditions, fmt.Sprintf("%s IN (%s)", column, strings.Join(placeholders, ", ")))
			}
		case "_nin":
			if arr, ok := value.([]interface{}); ok {
				placeholders := make([]string, len(arr))
				for i, v := range arr {
					placeholders[i] = "?"
					params = append(params, v)
				}
				conditions = append(conditions, fmt.Sprintf("%s NOT IN (%s)", column, strings.Join(placeholders, ", ")))
			}
		case "_like":
			conditions = append(conditions, fmt.Sprintf("%s LIKE ?", column))
			params = append(params, value)
		case "_nlike":
			conditions = append(conditions, fmt.Sprintf("%s NOT LIKE ?", column))
			params = append(params, value)
		case "_ilike":
			conditions = append(conditions, fmt.Sprintf("lower(%s) LIKE lower(?)", column))
			params = append(params, value)
		case "_is_null":
			if b, ok := value.(bool); ok && b {
				conditions = append(conditions, fmt.Sprintf("%s IS NULL", column))
			} else {
				conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", column))
			}
		}
	}

	return strings.Join(conditions, " AND "), params
}

// buildOrderByClause builds ORDER BY clause
func (e *Executor) buildOrderByClause(objType *ObjectType, orderBy interface{}) string {
	var orders []string

	switch v := orderBy.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				for field, dir := range m {
					if f, ok := objType.Fields[field]; ok {
						direction := "ASC"
						if dirStr, ok := dir.(string); ok {
							switch dirStr {
							case "desc", "desc_nulls_first", "desc_nulls_last":
								direction = "DESC"
							}
						}
						orders = append(orders, fmt.Sprintf("%s %s", f.ColumnName, direction))
					}
				}
			}
		}
	case map[string]interface{}:
		for field, dir := range v {
			if f, ok := objType.Fields[field]; ok {
				direction := "ASC"
				if dirStr, ok := dir.(string); ok {
					switch dirStr {
					case "desc", "desc_nulls_first", "desc_nulls_last":
						direction = "DESC"
					}
				}
				orders = append(orders, fmt.Sprintf("%s %s", f.ColumnName, direction))
			}
		}
	}

	return strings.Join(orders, ", ")
}

// executeAggregate executes an aggregate query
func (e *Executor) executeAggregate(ctx context.Context, field ParsedField, variables map[string]interface{}) (interface{}, error) {
	tableName := strings.TrimSuffix(field.Name, "_aggregate")
	qf := e.schema.GetQueryField(tableName)
	if qf == nil {
		return nil, fmt.Errorf("unknown table: %s", tableName)
	}

	objType := e.schema.GetType(qf.Type)
	if objType == nil {
		return nil, fmt.Errorf("unknown type: %s", qf.Type)
	}

	// Build COUNT query
	sql := fmt.Sprintf("SELECT count() as count FROM %s", tableName)
	var params []interface{}

	// Add WHERE clause if present
	if where, ok := field.Arguments["where"]; ok {
		if whereMap, ok := where.(map[string]interface{}); ok {
			whereClause, whereParams := e.buildWhereClause(objType, whereMap)
			if whereClause != "" {
				sql += " WHERE " + whereClause
				params = append(params, whereParams...)
			}
		}
	}

	rows, err := e.client.Query(ctx, sql, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var count int64
	if rows.Next() {
		rows.Scan(&count)
	}

	return map[string]interface{}{
		"aggregate": map[string]interface{}{
			"count": count,
		},
		"nodes": []interface{}{},
	}, nil
}

// executeIntrospection handles introspection queries
func (e *Executor) executeIntrospection(field ParsedField) (interface{}, error) {
	// Return basic schema introspection
	types := []map[string]interface{}{}

	for _, objType := range e.schema.Types() {
		fields := []map[string]interface{}{}
		for _, f := range objType.Fields {
			fields = append(fields, map[string]interface{}{
				"name": f.Name,
				"type": map[string]interface{}{
					"name": f.Type,
				},
			})
		}

		types = append(types, map[string]interface{}{
			"name":   objType.Name,
			"fields": fields,
		})
	}

	return map[string]interface{}{
		"types": types,
	}, nil
}
