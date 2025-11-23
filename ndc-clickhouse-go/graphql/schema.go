package graphql

import (
	"fmt"
	"strings"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
)

// Schema represents a GraphQL schema built from ClickHouse tables
type Schema struct {
	types       map[string]*ObjectType
	queryFields map[string]*QueryField
}

// ObjectType represents a GraphQL object type
type ObjectType struct {
	Name        string
	Description string
	Fields      map[string]*Field
	TableName   string // Original ClickHouse table name
}

// Field represents a GraphQL field
type Field struct {
	Name        string
	Type        string
	Description string
	Nullable    bool
	ColumnName  string // Original ClickHouse column name
	ColumnType  string // ClickHouse column type
}

// QueryField represents a query field (collection)
type QueryField struct {
	Name        string
	Type        string
	Description string
	TableName   string
	Arguments   map[string]*Argument
}

// Argument represents a GraphQL argument
type Argument struct {
	Name         string
	Type         string
	DefaultValue interface{}
	Description  string
}

// NewSchema creates a new empty schema
func NewSchema() *Schema {
	return &Schema{
		types:       make(map[string]*ObjectType),
		queryFields: make(map[string]*QueryField),
	}
}

// AddType adds a type from ClickHouse table
func (s *Schema) AddType(tableName string, columns []clickhouse.ColumnInfo) {
	typeName := toGraphQLTypeName(tableName)

	objType := &ObjectType{
		Name:      typeName,
		TableName: tableName,
		Fields:    make(map[string]*Field),
	}

	for _, col := range columns {
		fieldName := toGraphQLFieldName(col.Name)
		graphqlType, nullable := clickHouseToGraphQLType(col.Type)

		objType.Fields[fieldName] = &Field{
			Name:        fieldName,
			Type:        graphqlType,
			Nullable:    nullable,
			ColumnName:  col.Name,
			ColumnType:  col.Type,
			Description: col.Comment,
		}
	}

	s.types[typeName] = objType

	// Add query field for this type
	s.queryFields[tableName] = &QueryField{
		Name:      tableName,
		Type:      typeName,
		TableName: tableName,
		Arguments: map[string]*Argument{
			"limit": {
				Name:         "limit",
				Type:         "Int",
				DefaultValue: 100,
				Description:  "Maximum number of rows to return",
			},
			"offset": {
				Name:         "offset",
				Type:         "Int",
				DefaultValue: 0,
				Description:  "Number of rows to skip",
			},
			"where": {
				Name:        "where",
				Type:        typeName + "_bool_exp",
				Description: "Filter conditions",
			},
			"order_by": {
				Name:        "order_by",
				Type:        "[" + typeName + "_order_by!]",
				Description: "Sort order",
			},
		},
	}

	// Add aggregate query field
	s.queryFields[tableName+"_aggregate"] = &QueryField{
		Name:      tableName + "_aggregate",
		Type:      typeName + "_aggregate",
		TableName: tableName,
		Arguments: map[string]*Argument{
			"where": {
				Name:        "where",
				Type:        typeName + "_bool_exp",
				Description: "Filter conditions",
			},
		},
	}
}

// GetType returns a type by name
func (s *Schema) GetType(name string) *ObjectType {
	return s.types[name]
}

// GetQueryField returns a query field by name
func (s *Schema) GetQueryField(name string) *QueryField {
	return s.queryFields[name]
}

// Types returns all types
func (s *Schema) Types() map[string]*ObjectType {
	return s.types
}

// QueryFields returns all query fields
func (s *Schema) QueryFields() map[string]*QueryField {
	return s.queryFields
}

// SDL returns the schema in SDL format
func (s *Schema) SDL() string {
	var sb strings.Builder

	sb.WriteString("# ClickHouse GraphQL Schema\n")
	sb.WriteString("# Auto-generated from database introspection\n\n")

	// Scalar types
	sb.WriteString("scalar DateTime\n")
	sb.WriteString("scalar Date\n")
	sb.WriteString("scalar UUID\n")
	sb.WriteString("scalar JSON\n")
	sb.WriteString("scalar Decimal\n\n")

	// Enum for order direction
	sb.WriteString("enum order_by {\n")
	sb.WriteString("  asc\n")
	sb.WriteString("  asc_nulls_first\n")
	sb.WriteString("  asc_nulls_last\n")
	sb.WriteString("  desc\n")
	sb.WriteString("  desc_nulls_first\n")
	sb.WriteString("  desc_nulls_last\n")
	sb.WriteString("}\n\n")

	// Generate types for each table
	for _, objType := range s.types {
		// Main type
		sb.WriteString(fmt.Sprintf("type %s {\n", objType.Name))
		for _, field := range objType.Fields {
			typeStr := field.Type
			if !field.Nullable {
				typeStr += "!"
			}
			if field.Description != "" {
				sb.WriteString(fmt.Sprintf("  \"\"\"%s\"\"\"\n", field.Description))
			}
			sb.WriteString(fmt.Sprintf("  %s: %s\n", field.Name, typeStr))
		}
		sb.WriteString("}\n\n")

		// Bool expression type (for where clauses)
		sb.WriteString(fmt.Sprintf("input %s_bool_exp {\n", objType.Name))
		sb.WriteString("  _and: [" + objType.Name + "_bool_exp!]\n")
		sb.WriteString("  _or: [" + objType.Name + "_bool_exp!]\n")
		sb.WriteString("  _not: " + objType.Name + "_bool_exp\n")
		for _, field := range objType.Fields {
			compType := getComparisonType(field.Type)
			sb.WriteString(fmt.Sprintf("  %s: %s\n", field.Name, compType))
		}
		sb.WriteString("}\n\n")

		// Order by type
		sb.WriteString(fmt.Sprintf("input %s_order_by {\n", objType.Name))
		for _, field := range objType.Fields {
			sb.WriteString(fmt.Sprintf("  %s: order_by\n", field.Name))
		}
		sb.WriteString("}\n\n")

		// Aggregate type
		sb.WriteString(fmt.Sprintf("type %s_aggregate {\n", objType.Name))
		sb.WriteString("  aggregate: " + objType.Name + "_aggregate_fields\n")
		sb.WriteString("  nodes: [" + objType.Name + "!]!\n")
		sb.WriteString("}\n\n")

		sb.WriteString(fmt.Sprintf("type %s_aggregate_fields {\n", objType.Name))
		sb.WriteString("  count: Int!\n")
		// Add numeric aggregates for numeric fields
		for _, field := range objType.Fields {
			if isNumericType(field.Type) {
				sb.WriteString(fmt.Sprintf("  sum_%s: Float\n", field.Name))
				sb.WriteString(fmt.Sprintf("  avg_%s: Float\n", field.Name))
				sb.WriteString(fmt.Sprintf("  min_%s: %s\n", field.Name, field.Type))
				sb.WriteString(fmt.Sprintf("  max_%s: %s\n", field.Name, field.Type))
			}
		}
		sb.WriteString("}\n\n")
	}

	// Comparison input types
	sb.WriteString(generateComparisonTypes())

	// Query type
	sb.WriteString("type Query {\n")
	for _, qf := range s.queryFields {
		args := []string{}
		for _, arg := range qf.Arguments {
			argStr := fmt.Sprintf("%s: %s", arg.Name, arg.Type)
			if arg.DefaultValue != nil {
				argStr += fmt.Sprintf(" = %v", arg.DefaultValue)
			}
			args = append(args, argStr)
		}

		if strings.HasSuffix(qf.Name, "_aggregate") {
			sb.WriteString(fmt.Sprintf("  %s(%s): %s!\n", qf.Name, strings.Join(args, ", "), qf.Type))
		} else {
			sb.WriteString(fmt.Sprintf("  %s(%s): [%s!]!\n", qf.Name, strings.Join(args, ", "), qf.Type))
		}
	}
	sb.WriteString("}\n")

	return sb.String()
}

// Helper functions

func toGraphQLTypeName(name string) string {
	// Convert snake_case to PascalCase
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func toGraphQLFieldName(name string) string {
	// Keep as is for field names (snake_case is common in GraphQL)
	return name
}

func clickHouseToGraphQLType(chType string) (string, bool) {
	// Handle Nullable wrapper
	nullable := false
	if strings.HasPrefix(chType, "Nullable(") {
		nullable = true
		chType = chType[9 : len(chType)-1]
	}

	// Handle LowCardinality
	if strings.HasPrefix(chType, "LowCardinality(") {
		chType = chType[15 : len(chType)-1]
	}

	// Map ClickHouse types to GraphQL types
	chType = strings.ToLower(chType)

	switch {
	case strings.HasPrefix(chType, "string"), strings.HasPrefix(chType, "fixedstring"):
		return "String", nullable
	case strings.HasPrefix(chType, "int8"), strings.HasPrefix(chType, "int16"),
		strings.HasPrefix(chType, "int32"), strings.HasPrefix(chType, "uint8"),
		strings.HasPrefix(chType, "uint16"):
		return "Int", nullable
	case strings.HasPrefix(chType, "int64"), strings.HasPrefix(chType, "uint32"),
		strings.HasPrefix(chType, "uint64"), strings.HasPrefix(chType, "int128"),
		strings.HasPrefix(chType, "int256"), strings.HasPrefix(chType, "uint128"),
		strings.HasPrefix(chType, "uint256"):
		return "String", nullable // Use String for large integers
	case strings.HasPrefix(chType, "float32"), strings.HasPrefix(chType, "float64"):
		return "Float", nullable
	case strings.HasPrefix(chType, "decimal"):
		return "Decimal", nullable
	case strings.HasPrefix(chType, "bool"):
		return "Boolean", nullable
	case strings.HasPrefix(chType, "date32"), strings.HasPrefix(chType, "date"):
		return "Date", nullable
	case strings.HasPrefix(chType, "datetime64"), strings.HasPrefix(chType, "datetime"):
		return "DateTime", nullable
	case strings.HasPrefix(chType, "uuid"):
		return "UUID", nullable
	case strings.HasPrefix(chType, "array"):
		// Extract inner type
		inner := chType[6 : len(chType)-1]
		innerType, _ := clickHouseToGraphQLType(inner)
		return "[" + innerType + "]", nullable
	case strings.HasPrefix(chType, "map"), strings.HasPrefix(chType, "tuple"),
		strings.HasPrefix(chType, "json"):
		return "JSON", nullable
	case strings.HasPrefix(chType, "enum"):
		return "String", nullable
	case strings.HasPrefix(chType, "ipv4"), strings.HasPrefix(chType, "ipv6"):
		return "String", nullable
	default:
		return "String", nullable
	}
}

func getComparisonType(graphqlType string) string {
	// Remove array brackets if present
	baseType := strings.TrimPrefix(strings.TrimSuffix(graphqlType, "]"), "[")

	switch baseType {
	case "Int":
		return "Int_comparison_exp"
	case "Float", "Decimal":
		return "Float_comparison_exp"
	case "Boolean":
		return "Boolean_comparison_exp"
	case "String", "UUID":
		return "String_comparison_exp"
	case "DateTime", "Date":
		return "DateTime_comparison_exp"
	default:
		return "String_comparison_exp"
	}
}

func isNumericType(graphqlType string) bool {
	return graphqlType == "Int" || graphqlType == "Float" || graphqlType == "Decimal"
}

func generateComparisonTypes() string {
	return `
# Comparison expressions
input Int_comparison_exp {
  _eq: Int
  _neq: Int
  _gt: Int
  _gte: Int
  _lt: Int
  _lte: Int
  _in: [Int!]
  _nin: [Int!]
  _is_null: Boolean
}

input Float_comparison_exp {
  _eq: Float
  _neq: Float
  _gt: Float
  _gte: Float
  _lt: Float
  _lte: Float
  _in: [Float!]
  _nin: [Float!]
  _is_null: Boolean
}

input String_comparison_exp {
  _eq: String
  _neq: String
  _gt: String
  _gte: String
  _lt: String
  _lte: String
  _in: [String!]
  _nin: [String!]
  _like: String
  _nlike: String
  _ilike: String
  _nilike: String
  _regex: String
  _is_null: Boolean
}

input Boolean_comparison_exp {
  _eq: Boolean
  _neq: Boolean
  _is_null: Boolean
}

input DateTime_comparison_exp {
  _eq: DateTime
  _neq: DateTime
  _gt: DateTime
  _gte: DateTime
  _lt: DateTime
  _lte: DateTime
  _in: [DateTime!]
  _nin: [DateTime!]
  _is_null: Boolean
}

`
}
