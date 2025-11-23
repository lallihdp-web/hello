package duckdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Engine represents a query execution engine
type Engine string

const (
	EngineClickHouse Engine = "clickhouse"
	EngineDuckDB     Engine = "duckdb"
	EngineFederated  Engine = "federated"
)

// QueryRouter routes queries between ClickHouse and DuckDB
type QueryRouter struct {
	duckdb       *Client
	config       *HybridConfig
	catalog      *DataCatalog
	costModel    *CostModel
	stats        *RouterStats
	mu           sync.RWMutex
}

// ClickHouseExecutor interface for executing ClickHouse queries
type ClickHouseExecutor interface {
	Query(ctx context.Context, sql string, args ...interface{}) ([]map[string]interface{}, error)
	GetRowCount(ctx context.Context, collection string) (int64, error)
}

// NewQueryRouter creates a new hybrid query router
func NewQueryRouter(duckdb *Client, config *HybridConfig) *QueryRouter {
	if config == nil {
		config = &DefaultConfig().Hybrid
	}

	return &QueryRouter{
		duckdb:    duckdb,
		config:    config,
		catalog:   duckdb.catalog,
		costModel: NewCostModel(),
		stats:     &RouterStats{},
	}
}

// RouteQuery determines the best engine for a query
func (r *QueryRouter) RouteQuery(ctx context.Context, query *QueryPlan) (*RoutingDecision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	decision := &RoutingDecision{
		Query:      query,
		Timestamp:  time.Now(),
	}

	// Check if hybrid routing is enabled
	if !r.config.Enabled {
		decision.Engine = EngineClickHouse
		decision.Reason = "hybrid routing disabled"
		return decision, nil
	}

	// Check if collection is cached in DuckDB
	entry := r.catalog.GetCollectionInfo(query.Collection)
	if entry == nil || !entry.InDuckDB {
		decision.Engine = EngineClickHouse
		decision.Reason = "collection not cached in DuckDB"
		r.stats.ClickHouseQueries++
		return decision, nil
	}

	// Check data freshness
	if query.RequireFresh && !r.catalog.IsFresh(query.Collection) {
		decision.Engine = EngineClickHouse
		decision.Reason = "fresh data required, cache is stale"
		r.stats.ClickHouseQueries++
		return decision, nil
	}

	// Check max age requirement
	if query.MaxDataAge > 0 && time.Since(entry.LastUpdated) > query.MaxDataAge {
		decision.Engine = EngineClickHouse
		decision.Reason = fmt.Sprintf("data age %v exceeds max %v", time.Since(entry.LastUpdated), query.MaxDataAge)
		r.stats.ClickHouseQueries++
		return decision, nil
	}

	// Estimate costs
	if r.config.EnableQueryAnalysis {
		clickhouseCost := r.costModel.EstimateClickHouseCost(query, entry)
		duckdbCost := r.costModel.EstimateDuckDBCost(query, entry)

		decision.ClickHouseCost = clickhouseCost
		decision.DuckDBCost = duckdbCost

		// Use cost threshold to decide
		if duckdbCost < clickhouseCost*r.config.CostThreshold {
			decision.Engine = EngineDuckDB
			decision.Reason = fmt.Sprintf("DuckDB cost (%.2f) < ClickHouse cost (%.2f) * threshold (%.2f)",
				duckdbCost, clickhouseCost, r.config.CostThreshold)
			r.stats.DuckDBQueries++
			return decision, nil
		}
	}

	// Check row count preference
	if entry.RowCount < int64(r.config.PreferDuckDBUnderRows) {
		decision.Engine = EngineDuckDB
		decision.Reason = fmt.Sprintf("row count %d under threshold %d",
			entry.RowCount, r.config.PreferDuckDBUnderRows)
		r.stats.DuckDBQueries++
		return decision, nil
	}

	// Default to ClickHouse for large datasets
	decision.Engine = EngineClickHouse
	decision.Reason = "large dataset, prefer ClickHouse"
	r.stats.ClickHouseQueries++
	return decision, nil
}

// Execute runs a query using the appropriate engine
func (r *QueryRouter) Execute(ctx context.Context, query *QueryPlan, clickhouse ClickHouseExecutor) (*QueryResult, error) {
	// Get routing decision
	decision, err := r.RouteQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	result := &QueryResult{
		Decision: decision,
	}

	start := time.Now()

	switch decision.Engine {
	case EngineDuckDB:
		rows, err := r.executeDuckDB(ctx, query)
		if err != nil {
			// Fallback to ClickHouse on error if configured
			if r.config.FallbackOnError {
				r.stats.Fallbacks++
				decision.Engine = EngineClickHouse
				decision.Reason = fmt.Sprintf("DuckDB error, falling back: %v", err)
				rows, err = r.executeClickHouse(ctx, query, clickhouse)
			}
		}
		result.Rows = rows
		result.Error = err

	case EngineClickHouse:
		rows, err := r.executeClickHouse(ctx, query, clickhouse)
		result.Rows = rows
		result.Error = err

	case EngineFederated:
		rows, err := r.executeFederated(ctx, query, clickhouse)
		result.Rows = rows
		result.Error = err
	}

	result.Duration = time.Since(start)
	r.stats.TotalQueries++
	r.stats.TotalDuration += result.Duration

	return result, result.Error
}

// executeDuckDB runs a query against DuckDB
func (r *QueryRouter) executeDuckDB(ctx context.Context, query *QueryPlan) ([]map[string]interface{}, error) {
	sql := query.ToSQL()
	return r.duckdb.Query(ctx, sql)
}

// executeClickHouse runs a query against ClickHouse
func (r *QueryRouter) executeClickHouse(ctx context.Context, query *QueryPlan, executor ClickHouseExecutor) ([]map[string]interface{}, error) {
	sql := query.ToSQL()
	return executor.Query(ctx, sql)
}

// executeFederated runs a federated query across both engines
func (r *QueryRouter) executeFederated(ctx context.Context, query *QueryPlan, executor ClickHouseExecutor) ([]map[string]interface{}, error) {
	// For now, federated queries fall back to ClickHouse
	// Future: implement actual federation
	return r.executeClickHouse(ctx, query, executor)
}

// GetStats returns router statistics
func (r *QueryRouter) GetStats() RouterStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return *r.stats
}

// ResetStats resets router statistics
func (r *QueryRouter) ResetStats() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats = &RouterStats{}
}

// QueryPlan represents a parsed query
type QueryPlan struct {
	Collection   string                 `json:"collection"`
	Columns      []string               `json:"columns,omitempty"`
	Filters      map[string]interface{} `json:"filters,omitempty"`
	OrderBy      []OrderByClause        `json:"order_by,omitempty"`
	Limit        int                    `json:"limit,omitempty"`
	Offset       int                    `json:"offset,omitempty"`
	Aggregations []Aggregation          `json:"aggregations,omitempty"`
	GroupBy      []string               `json:"group_by,omitempty"`
	RequireFresh bool                   `json:"require_fresh,omitempty"`
	MaxDataAge   time.Duration          `json:"max_data_age,omitempty"`
}

// OrderByClause represents an ORDER BY clause
type OrderByClause struct {
	Column    string `json:"column"`
	Direction string `json:"direction"` // "asc" or "desc"
}

// Aggregation represents an aggregation function
type Aggregation struct {
	Function string `json:"function"` // sum, avg, count, min, max
	Column   string `json:"column"`
	Alias    string `json:"alias,omitempty"`
}

// ToSQL converts the query plan to SQL
func (q *QueryPlan) ToSQL() string {
	var sb strings.Builder

	// SELECT clause
	sb.WriteString("SELECT ")
	if len(q.Aggregations) > 0 {
		parts := make([]string, 0)
		for _, col := range q.GroupBy {
			parts = append(parts, col)
		}
		for _, agg := range q.Aggregations {
			alias := agg.Alias
			if alias == "" {
				alias = fmt.Sprintf("%s_%s", agg.Function, agg.Column)
			}
			parts = append(parts, fmt.Sprintf("%s(%s) AS %s", strings.ToUpper(agg.Function), agg.Column, alias))
		}
		sb.WriteString(strings.Join(parts, ", "))
	} else if len(q.Columns) > 0 {
		sb.WriteString(strings.Join(q.Columns, ", "))
	} else {
		sb.WriteString("*")
	}

	// FROM clause
	sb.WriteString(" FROM ")
	sb.WriteString(q.Collection)

	// WHERE clause
	if len(q.Filters) > 0 {
		sb.WriteString(" WHERE ")
		conditions := make([]string, 0, len(q.Filters))
		for col, val := range q.Filters {
			conditions = append(conditions, fmt.Sprintf("%s = %v", col, formatValue(val)))
		}
		sb.WriteString(strings.Join(conditions, " AND "))
	}

	// GROUP BY clause
	if len(q.GroupBy) > 0 {
		sb.WriteString(" GROUP BY ")
		sb.WriteString(strings.Join(q.GroupBy, ", "))
	}

	// ORDER BY clause
	if len(q.OrderBy) > 0 {
		sb.WriteString(" ORDER BY ")
		parts := make([]string, len(q.OrderBy))
		for i, ob := range q.OrderBy {
			parts[i] = fmt.Sprintf("%s %s", ob.Column, strings.ToUpper(ob.Direction))
		}
		sb.WriteString(strings.Join(parts, ", "))
	}

	// LIMIT/OFFSET
	if q.Limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", q.Limit))
	}
	if q.Offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", q.Offset))
	}

	return sb.String()
}

// RoutingDecision represents the routing decision for a query
type RoutingDecision struct {
	Engine         Engine     `json:"engine"`
	Reason         string     `json:"reason"`
	Query          *QueryPlan `json:"query"`
	ClickHouseCost float64    `json:"clickhouse_cost,omitempty"`
	DuckDBCost     float64    `json:"duckdb_cost,omitempty"`
	Timestamp      time.Time  `json:"timestamp"`
}

// QueryResult represents the result of a query execution
type QueryResult struct {
	Rows     []map[string]interface{} `json:"rows"`
	Decision *RoutingDecision         `json:"decision"`
	Duration time.Duration            `json:"duration"`
	Error    error                    `json:"error,omitempty"`
}

// RouterStats holds query routing statistics
type RouterStats struct {
	TotalQueries      int64         `json:"total_queries"`
	ClickHouseQueries int64         `json:"clickhouse_queries"`
	DuckDBQueries     int64         `json:"duckdb_queries"`
	FederatedQueries  int64         `json:"federated_queries"`
	Fallbacks         int64         `json:"fallbacks"`
	TotalDuration     time.Duration `json:"total_duration"`
}

// CostModel estimates query costs for routing decisions
type CostModel struct {
	// Cost factors
	NetworkLatencyMs     float64
	ClickHouseCostPerRow float64
	DuckDBCostPerRow     float64
	JoinCostFactor       float64
	AggregationCostFactor float64
}

// NewCostModel creates a new cost model with default values
func NewCostModel() *CostModel {
	return &CostModel{
		NetworkLatencyMs:      10,
		ClickHouseCostPerRow:  0.001,
		DuckDBCostPerRow:      0.0001,
		JoinCostFactor:        2.0,
		AggregationCostFactor: 1.5,
	}
}

// EstimateClickHouseCost estimates the cost of running a query on ClickHouse
func (m *CostModel) EstimateClickHouseCost(query *QueryPlan, entry *CatalogEntry) float64 {
	baseCost := m.NetworkLatencyMs + float64(entry.RowCount)*m.ClickHouseCostPerRow

	// Add aggregation cost
	if len(query.Aggregations) > 0 {
		baseCost *= m.AggregationCostFactor
	}

	return baseCost
}

// EstimateDuckDBCost estimates the cost of running a query on DuckDB
func (m *CostModel) EstimateDuckDBCost(query *QueryPlan, entry *CatalogEntry) float64 {
	// No network latency for local DuckDB
	baseCost := float64(entry.RowCount) * m.DuckDBCostPerRow

	// Add aggregation cost
	if len(query.Aggregations) > 0 {
		baseCost *= m.AggregationCostFactor
	}

	return baseCost
}

// Helper function to format values for SQL
func formatValue(val interface{}) string {
	switch v := val.(type) {
	case string:
		// Escape single quotes
		escaped := regexp.MustCompile(`'`).ReplaceAllString(v, "''")
		return fmt.Sprintf("'%s'", escaped)
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	default:
		return fmt.Sprintf("%v", v)
	}
}
