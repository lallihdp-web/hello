package schema

import (
	"testing"
)

func TestParseClickHouseType_Simple(t *testing.T) {
	tests := []struct {
		input    string
		expected TypeInfo
	}{
		{
			input:    "String",
			expected: TypeInfo{BaseType: "String", IsNullable: false, IsArray: false},
		},
		{
			input:    "Int32",
			expected: TypeInfo{BaseType: "Int32", IsNullable: false, IsArray: false},
		},
		{
			input:    "Float64",
			expected: TypeInfo{BaseType: "Float64", IsNullable: false, IsArray: false},
		},
		{
			input:    "UUID",
			expected: TypeInfo{BaseType: "UUID", IsNullable: false, IsArray: false},
		},
		{
			input:    "Bool",
			expected: TypeInfo{BaseType: "Bool", IsNullable: false, IsArray: false},
		},
	}

	for _, tt := range tests {
		result := ParseClickHouseType(tt.input)
		if result.BaseType != tt.expected.BaseType {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				tt.input, result.BaseType, tt.expected.BaseType)
		}
		if result.IsNullable != tt.expected.IsNullable {
			t.Errorf("ParseClickHouseType(%q).IsNullable = %v, want %v",
				tt.input, result.IsNullable, tt.expected.IsNullable)
		}
		if result.IsArray != tt.expected.IsArray {
			t.Errorf("ParseClickHouseType(%q).IsArray = %v, want %v",
				tt.input, result.IsArray, tt.expected.IsArray)
		}
	}
}

func TestParseClickHouseType_Nullable(t *testing.T) {
	tests := []struct {
		input    string
		expected TypeInfo
	}{
		{
			input:    "Nullable(String)",
			expected: TypeInfo{BaseType: "String", IsNullable: true, IsArray: false},
		},
		{
			input:    "Nullable(Int64)",
			expected: TypeInfo{BaseType: "Int64", IsNullable: true, IsArray: false},
		},
		{
			input:    "Nullable(UUID)",
			expected: TypeInfo{BaseType: "UUID", IsNullable: true, IsArray: false},
		},
	}

	for _, tt := range tests {
		result := ParseClickHouseType(tt.input)
		if result.BaseType != tt.expected.BaseType {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				tt.input, result.BaseType, tt.expected.BaseType)
		}
		if result.IsNullable != tt.expected.IsNullable {
			t.Errorf("ParseClickHouseType(%q).IsNullable = %v, want %v",
				tt.input, result.IsNullable, tt.expected.IsNullable)
		}
	}
}

func TestParseClickHouseType_Array(t *testing.T) {
	tests := []struct {
		input         string
		expectedArray bool
		expectedBase  string
	}{
		{
			input:         "Array(String)",
			expectedArray: true,
			expectedBase:  "Array",
		},
		{
			input:         "Array(Int32)",
			expectedArray: true,
			expectedBase:  "Array",
		},
	}

	for _, tt := range tests {
		result := ParseClickHouseType(tt.input)
		if result.IsArray != tt.expectedArray {
			t.Errorf("ParseClickHouseType(%q).IsArray = %v, want %v",
				tt.input, result.IsArray, tt.expectedArray)
		}
		if result.BaseType != tt.expectedBase {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				tt.input, result.BaseType, tt.expectedBase)
		}
	}
}

func TestParseClickHouseType_LowCardinality(t *testing.T) {
	result := ParseClickHouseType("LowCardinality(String)")

	if result.BaseType != "String" {
		t.Errorf("ParseClickHouseType(LowCardinality(String)).BaseType = %q, want %q",
			result.BaseType, "String")
	}
}

func TestParseClickHouseType_FixedString(t *testing.T) {
	result := ParseClickHouseType("FixedString(32)")

	if result.BaseType != ScalarString {
		t.Errorf("ParseClickHouseType(FixedString(32)).BaseType = %q, want %q",
			result.BaseType, ScalarString)
	}
}

func TestParseClickHouseType_Decimal(t *testing.T) {
	tests := []string{
		"Decimal(10, 2)",
		"Decimal32(2)",
		"Decimal64(4)",
		"Decimal128(6)",
	}

	for _, input := range tests {
		result := ParseClickHouseType(input)
		if result.BaseType != ScalarDecimal {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				input, result.BaseType, ScalarDecimal)
		}
	}
}

func TestParseClickHouseType_DateTime(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"DateTime", ScalarDateTime},
		{"DateTime('UTC')", ScalarDateTime},
		{"DateTime64(3)", ScalarDateTime64},
		{"DateTime64(3, 'UTC')", ScalarDateTime64},
	}

	for _, tt := range tests {
		result := ParseClickHouseType(tt.input)
		if result.BaseType != tt.expected {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				tt.input, result.BaseType, tt.expected)
		}
	}
}

func TestParseClickHouseType_Enum(t *testing.T) {
	tests := []string{
		"Enum8('a' = 1, 'b' = 2)",
		"Enum16('pending' = 1, 'done' = 2)",
	}

	for _, input := range tests {
		result := ParseClickHouseType(input)
		if result.BaseType != ScalarString {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				input, result.BaseType, ScalarString)
		}
	}
}

func TestParseClickHouseType_Complex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Map(String, Int64)", ScalarJSON},
		{"Tuple(String, Int64)", ScalarJSON},
		{"Nested(name String, value Int64)", ScalarJSON},
	}

	for _, tt := range tests {
		result := ParseClickHouseType(tt.input)
		if result.BaseType != tt.expected {
			t.Errorf("ParseClickHouseType(%q).BaseType = %q, want %q",
				tt.input, result.BaseType, tt.expected)
		}
	}
}

func TestClickHouseToScalarName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{ScalarInt8, "Int32"},
		{ScalarInt16, "Int32"},
		{ScalarInt32, "Int32"},
		{ScalarInt64, "Int64"},
		{ScalarUInt8, "Int32"},
		{ScalarUInt64, "Int64"},
		{ScalarFloat32, "Float32"},
		{ScalarFloat64, "Float64"},
		{ScalarString, "String"},
		{ScalarBool, "Boolean"},
		{ScalarUUID, "UUID"},
		{ScalarDate, "Date"},
		{ScalarDateTime, "DateTime"},
		{ScalarJSON, "JSON"},
		{"Unknown", "String"},
	}

	for _, tt := range tests {
		result := clickHouseToScalarName(tt.input)
		if result != tt.expected {
			t.Errorf("clickHouseToScalarName(%q) = %q, want %q",
				tt.input, result, tt.expected)
		}
	}
}

func TestGetScalarTypes(t *testing.T) {
	scalars := GetScalarTypes()

	requiredTypes := []string{
		"Int32", "Int64", "Float32", "Float64",
		"String", "Boolean", "Date", "DateTime", "UUID", "JSON",
	}

	for _, typeName := range requiredTypes {
		if _, ok := scalars[typeName]; !ok {
			t.Errorf("GetScalarTypes() missing type %q", typeName)
		}
	}

	// Check that String has comparison operators
	stringType := scalars["String"]
	expectedOps := []string{"_eq", "_neq", "_gt", "_gte", "_lt", "_lte", "_in", "_like"}
	for _, op := range expectedOps {
		if _, ok := stringType.ComparisonOperators[op]; !ok {
			t.Errorf("String scalar missing comparison operator %q", op)
		}
	}

	// Check that Int32 has aggregate functions
	int32Type := scalars["Int32"]
	expectedAggs := []string{"sum", "avg", "min", "max", "count"}
	for _, agg := range expectedAggs {
		if _, ok := int32Type.AggregateFunctions[agg]; !ok {
			t.Errorf("Int32 scalar missing aggregate function %q", agg)
		}
	}
}
