package config

import (
	"encoding/json"
	"fmt"
)

// PermissionsConfig holds all permission configurations
type PermissionsConfig struct {
	// Roles defines permissions for each role
	Roles map[string]RolePermissions `json:"roles"`

	// DefaultRole is used when no role is specified
	DefaultRole string `json:"default_role,omitempty"`

	// AdminRole has full access (bypasses all checks)
	AdminRole string `json:"admin_role,omitempty"`
}

// RolePermissions defines permissions for a single role
type RolePermissions struct {
	// Tables defines per-table permissions
	Tables map[string]TablePermission `json:"tables"`

	// InheritFrom allows inheriting permissions from another role
	InheritFrom string `json:"inherit_from,omitempty"`
}

// TablePermission defines permissions for a single table
type TablePermission struct {
	// Select permission for reading data
	Select *SelectPermission `json:"select,omitempty"`

	// Insert permission for creating data
	Insert *InsertPermission `json:"insert,omitempty"`

	// Update permission for modifying data (limited in ClickHouse)
	Update *UpdatePermission `json:"update,omitempty"`

	// Delete permission for removing data (limited in ClickHouse)
	Delete *DeletePermission `json:"delete,omitempty"`
}

// SelectPermission defines read permissions
type SelectPermission struct {
	// Columns that can be selected (empty = all columns)
	Columns []string `json:"columns,omitempty"`

	// Filter is a row-level security filter applied to all queries
	// Uses the same format as GraphQL where clauses
	Filter *PermissionFilter `json:"filter,omitempty"`

	// Limit maximum number of rows that can be returned
	Limit *int `json:"limit,omitempty"`

	// AllowAggregations allows aggregate queries
	AllowAggregations bool `json:"allow_aggregations,omitempty"`
}

// InsertPermission defines create permissions
type InsertPermission struct {
	// Columns that can be inserted
	Columns []string `json:"columns,omitempty"`

	// Check is a validation filter for new rows
	Check *PermissionFilter `json:"check,omitempty"`

	// Set defines columns that are automatically set on insert
	Set map[string]interface{} `json:"set,omitempty"`
}

// UpdatePermission defines modify permissions
type UpdatePermission struct {
	// Columns that can be updated
	Columns []string `json:"columns,omitempty"`

	// Filter restricts which rows can be updated
	Filter *PermissionFilter `json:"filter,omitempty"`

	// Set defines columns that are automatically set on update
	Set map[string]interface{} `json:"set,omitempty"`
}

// DeletePermission defines remove permissions
type DeletePermission struct {
	// Filter restricts which rows can be deleted
	Filter *PermissionFilter `json:"filter,omitempty"`
}

// PermissionFilter defines a filter condition for row-level security
type PermissionFilter struct {
	// And combines multiple conditions with AND
	And []PermissionFilter `json:"_and,omitempty"`

	// Or combines multiple conditions with OR
	Or []PermissionFilter `json:"_or,omitempty"`

	// Not negates a condition
	Not *PermissionFilter `json:"_not,omitempty"`

	// Column conditions (column_name -> operator -> value)
	// Example: {"user_id": {"_eq": "X-User-Id"}}
	Columns map[string]map[string]interface{} `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling for PermissionFilter
func (pf *PermissionFilter) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a map
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	pf.Columns = make(map[string]map[string]interface{})

	for key, value := range raw {
		switch key {
		case "_and":
			if err := json.Unmarshal(value, &pf.And); err != nil {
				return err
			}
		case "_or":
			if err := json.Unmarshal(value, &pf.Or); err != nil {
				return err
			}
		case "_not":
			pf.Not = &PermissionFilter{}
			if err := json.Unmarshal(value, pf.Not); err != nil {
				return err
			}
		default:
			// This is a column condition
			var operators map[string]interface{}
			if err := json.Unmarshal(value, &operators); err != nil {
				return err
			}
			pf.Columns[key] = operators
		}
	}

	return nil
}

// MarshalJSON implements custom JSON marshaling for PermissionFilter
func (pf PermissionFilter) MarshalJSON() ([]byte, error) {
	result := make(map[string]interface{})

	if len(pf.And) > 0 {
		result["_and"] = pf.And
	}
	if len(pf.Or) > 0 {
		result["_or"] = pf.Or
	}
	if pf.Not != nil {
		result["_not"] = pf.Not
	}
	for col, ops := range pf.Columns {
		result[col] = ops
	}

	return json.Marshal(result)
}

// SessionVariables holds session/request variables for permission evaluation
type SessionVariables map[string]interface{}

// Get retrieves a session variable by name
func (sv SessionVariables) Get(name string) (interface{}, bool) {
	// Handle X-* style variable names
	val, ok := sv[name]
	if ok {
		return val, true
	}

	// Try lowercase
	val, ok = sv[toLowerCase(name)]
	return val, ok
}

func toLowerCase(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32
		}
		result[i] = c
	}
	return string(result)
}

// PermissionChecker evaluates permissions for a given role and session
type PermissionChecker struct {
	config  *PermissionsConfig
	role    string
	session SessionVariables
}

// NewPermissionChecker creates a new permission checker
func NewPermissionChecker(config *PermissionsConfig, role string, session SessionVariables) *PermissionChecker {
	if role == "" {
		role = config.DefaultRole
	}
	return &PermissionChecker{
		config:  config,
		role:    role,
		session: session,
	}
}

// IsAdmin checks if the current role is the admin role
func (pc *PermissionChecker) IsAdmin() bool {
	return pc.config.AdminRole != "" && pc.role == pc.config.AdminRole
}

// CanSelect checks if the role can select from a table
func (pc *PermissionChecker) CanSelect(tableName string) bool {
	if pc.IsAdmin() {
		return true
	}

	perm := pc.getTablePermission(tableName)
	return perm != nil && perm.Select != nil
}

// GetSelectPermission returns the select permission for a table
func (pc *PermissionChecker) GetSelectPermission(tableName string) *SelectPermission {
	if pc.IsAdmin() {
		return &SelectPermission{AllowAggregations: true}
	}

	perm := pc.getTablePermission(tableName)
	if perm == nil {
		return nil
	}
	return perm.Select
}

// CanInsert checks if the role can insert into a table
func (pc *PermissionChecker) CanInsert(tableName string) bool {
	if pc.IsAdmin() {
		return true
	}

	perm := pc.getTablePermission(tableName)
	return perm != nil && perm.Insert != nil
}

// GetInsertPermission returns the insert permission for a table
func (pc *PermissionChecker) GetInsertPermission(tableName string) *InsertPermission {
	if pc.IsAdmin() {
		return &InsertPermission{}
	}

	perm := pc.getTablePermission(tableName)
	if perm == nil {
		return nil
	}
	return perm.Insert
}

// getTablePermission retrieves the table permission for the current role
func (pc *PermissionChecker) getTablePermission(tableName string) *TablePermission {
	rolePerms, ok := pc.config.Roles[pc.role]
	if !ok {
		return nil
	}

	// Check direct permission
	if perm, ok := rolePerms.Tables[tableName]; ok {
		return &perm
	}

	// Check inherited permission
	if rolePerms.InheritFrom != "" {
		inheritedPerms, ok := pc.config.Roles[rolePerms.InheritFrom]
		if ok {
			if perm, ok := inheritedPerms.Tables[tableName]; ok {
				return &perm
			}
		}
	}

	return nil
}

// BuildFilterSQL builds a SQL WHERE clause from a permission filter
func (pc *PermissionChecker) BuildFilterSQL(filter *PermissionFilter) (string, []interface{}, error) {
	if filter == nil {
		return "", nil, nil
	}

	return pc.buildFilterSQLInternal(filter)
}

func (pc *PermissionChecker) buildFilterSQLInternal(filter *PermissionFilter) (string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	// Handle AND
	if len(filter.And) > 0 {
		var andParts []string
		for _, f := range filter.And {
			sql, a, err := pc.buildFilterSQLInternal(&f)
			if err != nil {
				return "", nil, err
			}
			if sql != "" {
				andParts = append(andParts, "("+sql+")")
				args = append(args, a...)
			}
		}
		if len(andParts) > 0 {
			conditions = append(conditions, "("+join(andParts, " AND ")+")")
		}
	}

	// Handle OR
	if len(filter.Or) > 0 {
		var orParts []string
		for _, f := range filter.Or {
			sql, a, err := pc.buildFilterSQLInternal(&f)
			if err != nil {
				return "", nil, err
			}
			if sql != "" {
				orParts = append(orParts, "("+sql+")")
				args = append(args, a...)
			}
		}
		if len(orParts) > 0 {
			conditions = append(conditions, "("+join(orParts, " OR ")+")")
		}
	}

	// Handle NOT
	if filter.Not != nil {
		sql, a, err := pc.buildFilterSQLInternal(filter.Not)
		if err != nil {
			return "", nil, err
		}
		if sql != "" {
			conditions = append(conditions, "NOT ("+sql+")")
			args = append(args, a...)
		}
	}

	// Handle column conditions
	for col, operators := range filter.Columns {
		for op, value := range operators {
			sqlOp, err := pc.mapOperator(op)
			if err != nil {
				return "", nil, err
			}

			// Resolve session variables
			resolvedValue := pc.resolveValue(value)

			conditions = append(conditions, fmt.Sprintf("`%s` %s ?", col, sqlOp))
			args = append(args, resolvedValue)
		}
	}

	return join(conditions, " AND "), args, nil
}

func (pc *PermissionChecker) mapOperator(op string) (string, error) {
	switch op {
	case "_eq":
		return "=", nil
	case "_neq":
		return "!=", nil
	case "_gt":
		return ">", nil
	case "_gte":
		return ">=", nil
	case "_lt":
		return "<", nil
	case "_lte":
		return "<=", nil
	case "_in":
		return "IN", nil
	case "_like":
		return "LIKE", nil
	default:
		return "", fmt.Errorf("unsupported operator: %s", op)
	}
}

func (pc *PermissionChecker) resolveValue(value interface{}) interface{} {
	// Check if value is a session variable reference
	if strVal, ok := value.(string); ok {
		if len(strVal) > 2 && strVal[0] == 'X' && strVal[1] == '-' {
			// This looks like a session variable (X-*)
			if resolved, ok := pc.session.Get(strVal); ok {
				return resolved
			}
		}
	}
	return value
}

func join(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
