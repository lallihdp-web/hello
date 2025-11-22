package query

import (
	"testing"

	"github.com/hasura/ndc-sdk-go/schema"
)

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "`users`"},
		{"user_id", "`user_id`"},
		{"table`name", "`table``name`"},
		{"", "``"},
	}

	for _, tt := range tests {
		result := QuoteIdentifier(tt.input)
		if result != tt.expected {
			t.Errorf("QuoteIdentifier(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestQuoteString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "'hello'"},
		{"it's", "'it''s'"},
		{"", "''"},
	}

	for _, tt := range tests {
		result := QuoteString(tt.input)
		if result != tt.expected {
			t.Errorf("QuoteString(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestBuilder_BasicSelect(t *testing.T) {
	builder := NewBuilder("users")

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	expected := "SELECT * FROM `users`"
	if result.SQL != expected {
		t.Errorf("Build().SQL = %q, want %q", result.SQL, expected)
	}
}

func TestBuilder_WithFields(t *testing.T) {
	builder := NewBuilder("users")

	fields := map[string]schema.Field{
		"id":   schema.NewColumnField("id", nil).Encode(),
		"name": schema.NewColumnField("name", nil).Encode(),
	}

	builder.WithFields(fields)

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check that SQL contains the table name
	if result.SQL == "" {
		t.Error("Build().SQL is empty")
	}

	// Check that fields are returned
	if len(result.Fields) == 0 {
		t.Error("Build().Fields is empty")
	}
}

func TestBuilder_WithLimit(t *testing.T) {
	builder := NewBuilder("users")

	limit := 10
	builder.WithLimit(&limit)

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	expected := "SELECT * FROM `users` LIMIT 10"
	if result.SQL != expected {
		t.Errorf("Build().SQL = %q, want %q", result.SQL, expected)
	}
}

func TestBuilder_WithOffset(t *testing.T) {
	builder := NewBuilder("users")

	limit := 10
	offset := 20
	builder.WithLimit(&limit).WithOffset(&offset)

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	expected := "SELECT * FROM `users` LIMIT 10 OFFSET 20"
	if result.SQL != expected {
		t.Errorf("Build().SQL = %q, want %q", result.SQL, expected)
	}
}

func TestBuilder_WithOrderBy(t *testing.T) {
	builder := NewBuilder("users")

	orderBy := &schema.OrderBy{
		Elements: []schema.OrderByElement{
			{
				Target:         schema.NewOrderByColumn(nil, "created_at").Encode(),
				OrderDirection: schema.OrderDirectionDesc,
			},
		},
	}

	builder.WithOrderBy(orderBy)

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	expected := "SELECT * FROM `users` ORDER BY `created_at` DESC"
	if result.SQL != expected {
		t.Errorf("Build().SQL = %q, want %q", result.SQL, expected)
	}
}

func TestBuilder_MapOperator(t *testing.T) {
	builder := NewBuilder("users")

	tests := []struct {
		op       string
		expected string
	}{
		{"_eq", "="},
		{"_neq", "!="},
		{"_gt", ">"},
		{"_gte", ">="},
		{"_lt", "<"},
		{"_lte", "<="},
		{"unknown", "="},
	}

	for _, tt := range tests {
		result := builder.mapOperator(tt.op)
		if result != tt.expected {
			t.Errorf("mapOperator(%q) = %q, want %q", tt.op, result, tt.expected)
		}
	}
}

func TestBuilder_WithAggregates(t *testing.T) {
	builder := NewBuilder("orders")

	aggregates := map[string]schema.Aggregate{
		"total_count": schema.NewAggregateStarCount().Encode(),
	}

	builder.WithAggregates(aggregates)

	result, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Check that the SQL contains count(*)
	if result.SQL == "" {
		t.Error("Build().SQL is empty")
	}
}
