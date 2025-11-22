package schema

import (
	"regexp"
	"strings"

	"github.com/hasura/ndc-sdk-go/schema"
)

// ClickHouse scalar type names
const (
	ScalarInt8       = "Int8"
	ScalarInt16      = "Int16"
	ScalarInt32      = "Int32"
	ScalarInt64      = "Int64"
	ScalarInt128     = "Int128"
	ScalarInt256     = "Int256"
	ScalarUInt8      = "UInt8"
	ScalarUInt16     = "UInt16"
	ScalarUInt32     = "UInt32"
	ScalarUInt64     = "UInt64"
	ScalarUInt128    = "UInt128"
	ScalarUInt256    = "UInt256"
	ScalarFloat32    = "Float32"
	ScalarFloat64    = "Float64"
	ScalarDecimal    = "Decimal"
	ScalarString     = "String"
	ScalarFixedString = "FixedString"
	ScalarUUID       = "UUID"
	ScalarDate       = "Date"
	ScalarDate32     = "Date32"
	ScalarDateTime   = "DateTime"
	ScalarDateTime64 = "DateTime64"
	ScalarBool       = "Bool"
	ScalarJSON       = "JSON"
	ScalarIPv4       = "IPv4"
	ScalarIPv6       = "IPv6"
)

// Type patterns for parsing complex types
var (
	nullablePattern     = regexp.MustCompile(`^Nullable\((.+)\)$`)
	arrayPattern        = regexp.MustCompile(`^Array\((.+)\)$`)
	lowCardinalityPattern = regexp.MustCompile(`^LowCardinality\((.+)\)$`)
	fixedStringPattern  = regexp.MustCompile(`^FixedString\(\d+\)$`)
	decimalPattern      = regexp.MustCompile(`^Decimal\d*\(\d+,\s*\d+\)$`)
	dateTimePattern     = regexp.MustCompile(`^DateTime64?\(.*\)$`)
	enumPattern         = regexp.MustCompile(`^Enum(?:8|16)?\(.*\)$`)
	mapPattern          = regexp.MustCompile(`^Map\((.+),\s*(.+)\)$`)
	tuplePattern        = regexp.MustCompile(`^Tuple\((.+)\)$`)
	nestedPattern       = regexp.MustCompile(`^Nested\((.+)\)$`)
)

// TypeInfo holds parsed type information
type TypeInfo struct {
	BaseType   string
	IsNullable bool
	IsArray    bool
	ArrayType  *TypeInfo
	MapKey     *TypeInfo
	MapValue   *TypeInfo
}

// ParseClickHouseType parses a ClickHouse type string
func ParseClickHouseType(typeStr string) TypeInfo {
	info := TypeInfo{BaseType: typeStr}

	// Handle Nullable
	if matches := nullablePattern.FindStringSubmatch(typeStr); len(matches) == 2 {
		info.IsNullable = true
		inner := ParseClickHouseType(matches[1])
		info.BaseType = inner.BaseType
		info.IsArray = inner.IsArray
		info.ArrayType = inner.ArrayType
		return info
	}

	// Handle LowCardinality (unwrap and continue)
	if matches := lowCardinalityPattern.FindStringSubmatch(typeStr); len(matches) == 2 {
		inner := ParseClickHouseType(matches[1])
		return inner
	}

	// Handle Array
	if matches := arrayPattern.FindStringSubmatch(typeStr); len(matches) == 2 {
		info.IsArray = true
		inner := ParseClickHouseType(matches[1])
		info.ArrayType = &inner
		info.BaseType = "Array"
		return info
	}

	// Handle FixedString
	if fixedStringPattern.MatchString(typeStr) {
		info.BaseType = ScalarString
		return info
	}

	// Handle Decimal variants
	if decimalPattern.MatchString(typeStr) || strings.HasPrefix(typeStr, "Decimal") {
		info.BaseType = ScalarDecimal
		return info
	}

	// Handle DateTime variants
	if dateTimePattern.MatchString(typeStr) || strings.HasPrefix(typeStr, "DateTime") {
		if strings.HasPrefix(typeStr, "DateTime64") {
			info.BaseType = ScalarDateTime64
		} else {
			info.BaseType = ScalarDateTime
		}
		return info
	}

	// Handle Enum (treat as String)
	if enumPattern.MatchString(typeStr) {
		info.BaseType = ScalarString
		return info
	}

	// Handle Map (treat as JSON)
	if mapPattern.MatchString(typeStr) {
		info.BaseType = ScalarJSON
		return info
	}

	// Handle Tuple (treat as JSON)
	if tuplePattern.MatchString(typeStr) {
		info.BaseType = ScalarJSON
		return info
	}

	// Handle Nested (treat as JSON array)
	if nestedPattern.MatchString(typeStr) {
		info.BaseType = ScalarJSON
		info.IsArray = true
		return info
	}

	return info
}

// ToNDCType converts a ClickHouse type to an NDC type
func ToNDCType(typeStr string) schema.Type {
	info := ParseClickHouseType(typeStr)
	return infoToNDCType(info)
}

func infoToNDCType(info TypeInfo) schema.Type {
	var baseType schema.Type

	if info.IsArray && info.ArrayType != nil {
		innerType := infoToNDCType(*info.ArrayType)
		baseType = schema.NewArrayType(innerType)
	} else {
		scalarName := clickHouseToScalarName(info.BaseType)
		baseType = schema.NewNamedType(scalarName)
	}

	if info.IsNullable {
		return schema.NewNullableType(baseType)
	}
	return baseType
}

// clickHouseToScalarName maps ClickHouse type names to NDC scalar names
func clickHouseToScalarName(chType string) string {
	switch chType {
	case ScalarInt8, ScalarInt16, ScalarInt32:
		return "Int32"
	case ScalarInt64, ScalarInt128, ScalarInt256:
		return "Int64"
	case ScalarUInt8, ScalarUInt16, ScalarUInt32:
		return "Int32"
	case ScalarUInt64, ScalarUInt128, ScalarUInt256:
		return "Int64"
	case ScalarFloat32:
		return "Float32"
	case ScalarFloat64, ScalarDecimal:
		return "Float64"
	case ScalarString, ScalarFixedString:
		return "String"
	case ScalarUUID:
		return "UUID"
	case ScalarDate, ScalarDate32:
		return "Date"
	case ScalarDateTime, ScalarDateTime64:
		return "DateTime"
	case ScalarBool:
		return "Boolean"
	case ScalarJSON:
		return "JSON"
	case ScalarIPv4, ScalarIPv6:
		return "String"
	default:
		return "String"
	}
}

// GetScalarTypes returns all supported scalar types with their representations
func GetScalarTypes() schema.SchemaResponseScalarTypes {
	return schema.SchemaResponseScalarTypes{
		"Int32": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"sum":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
				"avg":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int32")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int32")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getNumericComparisonOperators(),
			Representation:      schema.NewTypeRepresentationInt32().Encode(),
		},
		"Int64": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"sum":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
				"avg":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getNumericComparisonOperators(),
			Representation:      schema.NewTypeRepresentationInt64().Encode(),
		},
		"Float32": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"sum":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"avg":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float32")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float32")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getNumericComparisonOperators(),
			Representation:      schema.NewTypeRepresentationFloat32().Encode(),
		},
		"Float64": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"sum":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"avg":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Float64")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getNumericComparisonOperators(),
			Representation:      schema.NewTypeRepresentationFloat64().Encode(),
		},
		"String": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("String")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("String")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getStringComparisonOperators(),
			Representation:      schema.NewTypeRepresentationString().Encode(),
		},
		"Boolean": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: schema.ScalarTypeComparisonOperators{
				"_eq": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeEqual},
				"_neq": schema.ComparisonOperatorDefinition{
					Type:                 schema.ComparisonOperatorDefinitionTypeCustom,
					ArgumentType:         schema.NewNamedType("Boolean").Encode(),
				},
			},
			Representation: schema.NewTypeRepresentationBoolean().Encode(),
		},
		"Date": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Date")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Date")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getDateComparisonOperators("Date"),
			Representation:      schema.NewTypeRepresentationDate().Encode(),
		},
		"DateTime": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"min":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("DateTime")},
				"max":   schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("DateTime")},
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: getDateComparisonOperators("DateTime"),
			Representation:      schema.NewTypeRepresentationTimestamp().Encode(),
		},
		"UUID": schema.ScalarType{
			AggregateFunctions: schema.ScalarTypeAggregateFunctions{
				"count": schema.AggregateFunctionDefinition{ResultType: schema.NewNamedType("Int64")},
			},
			ComparisonOperators: schema.ScalarTypeComparisonOperators{
				"_eq":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeEqual},
				"_neq": schema.ComparisonOperatorDefinition{
					Type:         schema.ComparisonOperatorDefinitionTypeCustom,
					ArgumentType: schema.NewNamedType("UUID").Encode(),
				},
				"_in": schema.ComparisonOperatorDefinition{
					Type:         schema.ComparisonOperatorDefinitionTypeIn,
				},
			},
			Representation: schema.NewTypeRepresentationUUID().Encode(),
		},
		"JSON": schema.ScalarType{
			AggregateFunctions:  schema.ScalarTypeAggregateFunctions{},
			ComparisonOperators: schema.ScalarTypeComparisonOperators{},
			Representation:      schema.NewTypeRepresentationJSON().Encode(),
		},
	}
}

func getNumericComparisonOperators() schema.ScalarTypeComparisonOperators {
	return schema.ScalarTypeComparisonOperators{
		"_eq":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeEqual},
		"_neq": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("Int64").Encode()},
		"_gt":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("Int64").Encode()},
		"_gte": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("Int64").Encode()},
		"_lt":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("Int64").Encode()},
		"_lte": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("Int64").Encode()},
		"_in":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeIn},
	}
}

func getStringComparisonOperators() schema.ScalarTypeComparisonOperators {
	return schema.ScalarTypeComparisonOperators{
		"_eq":       schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeEqual},
		"_neq":      schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_gt":       schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_gte":      schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_lt":       schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_lte":      schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_in":       schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeIn},
		"_like":     schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_ilike":    schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_regex":    schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
		"_iregex":   schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType("String").Encode()},
	}
}

func getDateComparisonOperators(typeName string) schema.ScalarTypeComparisonOperators {
	return schema.ScalarTypeComparisonOperators{
		"_eq":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeEqual},
		"_neq": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType(typeName).Encode()},
		"_gt":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType(typeName).Encode()},
		"_gte": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType(typeName).Encode()},
		"_lt":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType(typeName).Encode()},
		"_lte": schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeCustom, ArgumentType: schema.NewNamedType(typeName).Encode()},
		"_in":  schema.ComparisonOperatorDefinition{Type: schema.ComparisonOperatorDefinitionTypeIn},
	}
}
