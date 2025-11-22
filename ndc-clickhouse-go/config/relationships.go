package config

// Relationship defines a relationship between two tables
type Relationship struct {
	// Name of the relationship (used in GraphQL)
	Name string `json:"name"`

	// Type of relationship: "object" (many-to-one) or "array" (one-to-many)
	Type RelationshipType `json:"type"`

	// Source table name
	SourceTable string `json:"source_table"`

	// Target table name
	TargetTable string `json:"target_table"`

	// Column mappings from source to target
	ColumnMapping map[string]string `json:"column_mapping"`

	// Description for documentation
	Description string `json:"description,omitempty"`
}

// RelationshipType defines the cardinality of a relationship
type RelationshipType string

const (
	// ObjectRelationship is a many-to-one relationship (foreign key)
	ObjectRelationship RelationshipType = "object"

	// ArrayRelationship is a one-to-many relationship
	ArrayRelationship RelationshipType = "array"
)

// RelationshipsConfig holds all relationship configurations
type RelationshipsConfig struct {
	// Relationships list
	Relationships []Relationship `json:"relationships"`

	// AutoDetect enables automatic relationship detection based on naming conventions
	AutoDetect bool `json:"auto_detect,omitempty"`

	// ForeignKeyPattern is the pattern for detecting foreign keys (default: "{table}_id")
	ForeignKeyPattern string `json:"foreign_key_pattern,omitempty"`
}

// GetRelationshipsForTable returns all relationships where the given table is the source
func (rc *RelationshipsConfig) GetRelationshipsForTable(tableName string) []Relationship {
	var result []Relationship
	for _, rel := range rc.Relationships {
		if rel.SourceTable == tableName {
			result = append(result, rel)
		}
	}
	return result
}

// GetReverseRelationshipsForTable returns all relationships where the given table is the target
func (rc *RelationshipsConfig) GetReverseRelationshipsForTable(tableName string) []Relationship {
	var result []Relationship
	for _, rel := range rc.Relationships {
		if rel.TargetTable == tableName {
			result = append(result, rel)
		}
	}
	return result
}
