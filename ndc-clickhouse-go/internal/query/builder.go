package query

import (
	"fmt"
	"strings"

	"github.com/hasura/ndc-sdk-go/schema"
)

// Builder constructs ClickHouse SQL queries from NDC query requests
type Builder struct {
	collection string
	fields     map[string]schema.Field
	where      *schema.Expression
	orderBy    *schema.OrderBy
	limit      *int
	offset     *int
	aggregates map[string]schema.Aggregate
	variables  map[string]interface{}
}

// NewBuilder creates a new query builder
func NewBuilder(collection string) *Builder {
	return &Builder{
		collection: collection,
		fields:     make(map[string]schema.Field),
		aggregates: make(map[string]schema.Aggregate),
		variables:  make(map[string]interface{}),
	}
}

// WithFields sets the fields to select
func (b *Builder) WithFields(fields map[string]schema.Field) *Builder {
	b.fields = fields
	return b
}

// WithWhere sets the where clause
func (b *Builder) WithWhere(expr *schema.Expression) *Builder {
	b.where = expr
	return b
}

// WithOrderBy sets the order by clause
func (b *Builder) WithOrderBy(orderBy *schema.OrderBy) *Builder {
	b.orderBy = orderBy
	return b
}

// WithLimit sets the limit
func (b *Builder) WithLimit(limit *int) *Builder {
	b.limit = limit
	return b
}

// WithOffset sets the offset
func (b *Builder) WithOffset(offset *int) *Builder {
	b.offset = offset
	return b
}

// WithAggregates sets the aggregates to compute
func (b *Builder) WithAggregates(aggregates map[string]schema.Aggregate) *Builder {
	b.aggregates = aggregates
	return b
}

// WithVariables sets query variables
func (b *Builder) WithVariables(variables map[string]interface{}) *Builder {
	b.variables = variables
	return b
}

// BuildResult holds the built query and parameters
type BuildResult struct {
	SQL    string
	Args   []interface{}
	Fields []string
}

// Build constructs the SQL query
func (b *Builder) Build() (*BuildResult, error) {
	result := &BuildResult{
		Args: make([]interface{}),
	}

	var sql strings.Builder

	// SELECT clause
	selectCols, fields, err := b.buildSelect()
	if err != nil {
		return nil, fmt.Errorf("failed to build SELECT: %w", err)
	}
	result.Fields = fields

	sql.WriteString("SELECT ")
	sql.WriteString(selectCols)
	sql.WriteString(" FROM ")
	sql.WriteString(QuoteIdentifier(b.collection))

	// WHERE clause
	if b.where != nil {
		whereClause, whereArgs, err := b.buildWhere(b.where)
		if err != nil {
			return nil, fmt.Errorf("failed to build WHERE: %w", err)
		}
		if whereClause != "" {
			sql.WriteString(" WHERE ")
			sql.WriteString(whereClause)
			result.Args = append(result.Args, whereArgs...)
		}
	}

	// ORDER BY clause
	if b.orderBy != nil && len(b.orderBy.Elements) > 0 {
		orderClause, err := b.buildOrderBy()
		if err != nil {
			return nil, fmt.Errorf("failed to build ORDER BY: %w", err)
		}
		sql.WriteString(" ORDER BY ")
		sql.WriteString(orderClause)
	}

	// LIMIT and OFFSET
	if b.limit != nil {
		sql.WriteString(fmt.Sprintf(" LIMIT %d", *b.limit))
	}
	if b.offset != nil {
		sql.WriteString(fmt.Sprintf(" OFFSET %d", *b.offset))
	}

	result.SQL = sql.String()
	return result, nil
}

// buildSelect constructs the SELECT clause
func (b *Builder) buildSelect() (string, []string, error) {
	var cols []string
	var fields []string

	// If we have aggregates, build aggregate query
	if len(b.aggregates) > 0 {
		for alias, agg := range b.aggregates {
			col, err := b.buildAggregate(alias, agg)
			if err != nil {
				return "", nil, err
			}
			cols = append(cols, col)
			fields = append(fields, alias)
		}
		return strings.Join(cols, ", "), fields, nil
	}

	// Regular field selection
	if len(b.fields) == 0 {
		return "*", []string{"*"}, nil
	}

	for alias, field := range b.fields {
		switch f := field.Interface().(type) {
		case *schema.ColumnField:
			col := QuoteIdentifier(f.Column)
			if alias != f.Column {
				col = fmt.Sprintf("%s AS %s", col, QuoteIdentifier(alias))
			}
			cols = append(cols, col)
			fields = append(fields, alias)
		case *schema.RelationshipField:
			// Relationships are handled separately
			continue
		}
	}

	if len(cols) == 0 {
		return "*", []string{"*"}, nil
	}

	return strings.Join(cols, ", "), fields, nil
}

// buildWhere constructs the WHERE clause
func (b *Builder) buildWhere(expr *schema.Expression) (string, []interface{}, error) {
	if expr == nil {
		return "", nil, nil
	}

	switch e := expr.Interface().(type) {
	case *schema.ExpressionAnd:
		return b.buildAndExpression(e.Expressions)
	case *schema.ExpressionOr:
		return b.buildOrExpression(e.Expressions)
	case *schema.ExpressionNot:
		return b.buildNotExpression(e.Expression)
	case *schema.ExpressionUnaryComparisonOperator:
		return b.buildUnaryComparison(e)
	case *schema.ExpressionBinaryComparisonOperator:
		return b.buildBinaryComparison(e)
	default:
		return "", nil, fmt.Errorf("unsupported expression type: %T", expr.Interface())
	}
}

func (b *Builder) buildAndExpression(exprs []schema.Expression) (string, []interface{}, error) {
	if len(exprs) == 0 {
		return "", nil, nil
	}

	var parts []string
	var args []interface{}

	for _, expr := range exprs {
		part, partArgs, err := b.buildWhere(&expr)
		if err != nil {
			return "", nil, err
		}
		if part != "" {
			parts = append(parts, "("+part+")")
			args = append(args, partArgs...)
		}
	}

	if len(parts) == 0 {
		return "", nil, nil
	}

	return strings.Join(parts, " AND "), args, nil
}

func (b *Builder) buildOrExpression(exprs []schema.Expression) (string, []interface{}, error) {
	if len(exprs) == 0 {
		return "", nil, nil
	}

	var parts []string
	var args []interface{}

	for _, expr := range exprs {
		part, partArgs, err := b.buildWhere(&expr)
		if err != nil {
			return "", nil, err
		}
		if part != "" {
			parts = append(parts, "("+part+")")
			args = append(args, partArgs...)
		}
	}

	if len(parts) == 0 {
		return "", nil, nil
	}

	return "(" + strings.Join(parts, " OR ") + ")", args, nil
}

func (b *Builder) buildNotExpression(expr schema.Expression) (string, []interface{}, error) {
	part, args, err := b.buildWhere(&expr)
	if err != nil {
		return "", nil, err
	}
	if part == "" {
		return "", nil, nil
	}

	return "NOT (" + part + ")", args, nil
}

func (b *Builder) buildUnaryComparison(e *schema.ExpressionUnaryComparisonOperator) (string, []interface{}, error) {
	col, err := b.buildComparisonTarget(e.Column)
	if err != nil {
		return "", nil, err
	}

	switch e.Operator {
	case schema.UnaryComparisonOperatorIsNull:
		return col + " IS NULL", nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported unary operator: %s", e.Operator)
	}
}

func (b *Builder) buildBinaryComparison(e *schema.ExpressionBinaryComparisonOperator) (string, []interface{}, error) {
	col, err := b.buildComparisonTarget(e.Column)
	if err != nil {
		return "", nil, err
	}

	value, args, err := b.buildComparisonValue(e.Value)
	if err != nil {
		return "", nil, err
	}

	op := b.mapOperator(e.Operator)

	// Handle special operators
	switch e.Operator {
	case "_in":
		return fmt.Sprintf("%s IN (%s)", col, value), args, nil
	case "_like":
		return fmt.Sprintf("%s LIKE %s", col, value), args, nil
	case "_ilike":
		return fmt.Sprintf("lower(%s) LIKE lower(%s)", col, value), args, nil
	case "_regex":
		return fmt.Sprintf("match(%s, %s)", col, value), args, nil
	case "_iregex":
		return fmt.Sprintf("match(lower(%s), lower(%s))", col, value), args, nil
	default:
		return fmt.Sprintf("%s %s %s", col, op, value), args, nil
	}
}

func (b *Builder) buildComparisonTarget(target schema.ComparisonTarget) (string, error) {
	switch t := target.Interface().(type) {
	case *schema.ComparisonTargetColumn:
		// Handle nested path
		if len(t.Path) > 0 {
			return "", fmt.Errorf("relationship paths not yet supported")
		}
		return QuoteIdentifier(t.Name), nil
	case *schema.ComparisonTargetRootCollectionColumn:
		return QuoteIdentifier(t.Name), nil
	default:
		return "", fmt.Errorf("unsupported comparison target type: %T", target.Interface())
	}
}

func (b *Builder) buildComparisonValue(value schema.ComparisonValue) (string, []interface{}, error) {
	switch v := value.Interface().(type) {
	case *schema.ComparisonValueColumn:
		col, err := b.buildComparisonTarget(v.Column)
		return col, nil, err
	case *schema.ComparisonValueScalar:
		return "?", []interface{}{v.Value}, nil
	case *schema.ComparisonValueVariable:
		if val, ok := b.variables[v.Name]; ok {
			return "?", []interface{}{val}, nil
		}
		return "", nil, fmt.Errorf("variable not found: %s", v.Name)
	default:
		return "", nil, fmt.Errorf("unsupported comparison value type: %T", value.Interface())
	}
}

func (b *Builder) mapOperator(op string) string {
	switch op {
	case "_eq":
		return "="
	case "_neq":
		return "!="
	case "_gt":
		return ">"
	case "_gte":
		return ">="
	case "_lt":
		return "<"
	case "_lte":
		return "<="
	default:
		return "="
	}
}

// buildOrderBy constructs the ORDER BY clause
func (b *Builder) buildOrderBy() (string, error) {
	var parts []string

	for _, elem := range b.orderBy.Elements {
		col, err := b.buildOrderByTarget(elem.Target)
		if err != nil {
			return "", err
		}

		dir := "ASC"
		if elem.OrderDirection == schema.OrderDirectionDesc {
			dir = "DESC"
		}

		parts = append(parts, fmt.Sprintf("%s %s", col, dir))
	}

	return strings.Join(parts, ", "), nil
}

func (b *Builder) buildOrderByTarget(target schema.OrderByTarget) (string, error) {
	switch t := target.Interface().(type) {
	case *schema.OrderByColumn:
		return QuoteIdentifier(t.Name), nil
	case *schema.OrderBySingleColumnAggregate:
		return fmt.Sprintf("%s(%s)", t.Function, QuoteIdentifier(t.Column)), nil
	case *schema.OrderByStarCountAggregate:
		return "count(*)", nil
	default:
		return "", fmt.Errorf("unsupported order by target type: %T", target.Interface())
	}
}

// buildAggregate constructs an aggregate expression
func (b *Builder) buildAggregate(alias string, agg schema.Aggregate) (string, error) {
	switch a := agg.Interface().(type) {
	case *schema.AggregateStarCount:
		return fmt.Sprintf("count(*) AS %s", QuoteIdentifier(alias)), nil
	case *schema.AggregateColumnCount:
		if a.Distinct {
			return fmt.Sprintf("count(DISTINCT %s) AS %s", QuoteIdentifier(a.Column), QuoteIdentifier(alias)), nil
		}
		return fmt.Sprintf("count(%s) AS %s", QuoteIdentifier(a.Column), QuoteIdentifier(alias)), nil
	case *schema.AggregateSingleColumn:
		return fmt.Sprintf("%s(%s) AS %s", a.Function, QuoteIdentifier(a.Column), QuoteIdentifier(alias)), nil
	default:
		return "", fmt.Errorf("unsupported aggregate type: %T", agg.Interface())
	}
}

// QuoteIdentifier quotes a ClickHouse identifier
func QuoteIdentifier(name string) string {
	// ClickHouse uses backticks or double quotes for identifiers
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// QuoteString quotes a string value
func QuoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
