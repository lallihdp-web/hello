package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// QueryLogger logs and analyzes queries
type QueryLogger struct {
	enabled      bool
	logFile      *os.File
	mu           sync.Mutex
	queries      []QueryLog
	maxInMemory  int
	slowThreshold time.Duration

	// Statistics
	stats QueryStats
	statsMu sync.RWMutex
}

// QueryLog represents a logged query
type QueryLog struct {
	ID          string                 `json:"id"`
	Collection  string                 `json:"collection"`
	Operation   string                 `json:"operation"` // query, mutation
	SQL         string                 `json:"sql,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
	Duration    time.Duration          `json:"duration"`
	RowCount    int                    `json:"row_count"`
	Error       string                 `json:"error,omitempty"`
	Role        string                 `json:"role,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	IsSlow      bool                   `json:"is_slow"`
	CacheHit    bool                   `json:"cache_hit"`
}

// QueryStats holds query statistics
type QueryStats struct {
	TotalQueries    int64         `json:"total_queries"`
	TotalErrors     int64         `json:"total_errors"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
	SlowQueries     int64         `json:"slow_queries"`
	CacheHits       int64         `json:"cache_hits"`
	CacheMisses     int64         `json:"cache_misses"`

	// Per-collection stats
	CollectionStats map[string]*CollectionStats `json:"collection_stats"`

	// Per-operation stats
	OperationStats map[string]*OperationStats `json:"operation_stats"`

	// Time-based stats
	QueriesPerMinute float64 `json:"queries_per_minute"`
	ErrorRate        float64 `json:"error_rate"`

	StartTime time.Time `json:"start_time"`
}

// CollectionStats holds per-collection statistics
type CollectionStats struct {
	QueryCount      int64         `json:"query_count"`
	ErrorCount      int64         `json:"error_count"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
}

// OperationStats holds per-operation statistics
type OperationStats struct {
	Count           int64         `json:"count"`
	ErrorCount      int64         `json:"error_count"`
	TotalDuration   time.Duration `json:"total_duration"`
	AverageDuration time.Duration `json:"average_duration"`
}

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Enabled       bool          `json:"enabled"`
	LogFile       string        `json:"log_file,omitempty"`
	MaxInMemory   int           `json:"max_in_memory"`
	SlowThreshold time.Duration `json:"slow_threshold"`
}

// DefaultLoggerConfig returns default logger configuration
func DefaultLoggerConfig() *LoggerConfig {
	return &LoggerConfig{
		Enabled:       true,
		MaxInMemory:   10000,
		SlowThreshold: time.Second,
	}
}

// NewQueryLogger creates a new query logger
func NewQueryLogger(cfg *LoggerConfig) (*QueryLogger, error) {
	if cfg == nil {
		cfg = DefaultLoggerConfig()
	}

	logger := &QueryLogger{
		enabled:       cfg.Enabled,
		maxInMemory:   cfg.MaxInMemory,
		slowThreshold: cfg.SlowThreshold,
		queries:       make([]QueryLog, 0),
		stats: QueryStats{
			CollectionStats: make(map[string]*CollectionStats),
			OperationStats:  make(map[string]*OperationStats),
			StartTime:       time.Now(),
		},
	}

	// Open log file if specified
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		logger.logFile = f
	}

	return logger, nil
}

// Log logs a query
func (l *QueryLogger) Log(log QueryLog) {
	if !l.enabled {
		return
	}

	log.Timestamp = time.Now()
	log.IsSlow = log.Duration >= l.slowThreshold

	// Generate ID if not set
	if log.ID == "" {
		log.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	// Store in memory
	l.mu.Lock()
	l.queries = append(l.queries, log)
	if len(l.queries) > l.maxInMemory {
		l.queries = l.queries[1:]
	}
	l.mu.Unlock()

	// Write to file
	if l.logFile != nil {
		data, _ := json.Marshal(log)
		l.logFile.Write(append(data, '\n'))
	}

	// Update stats
	l.updateStats(log)
}

// updateStats updates the statistics
func (l *QueryLogger) updateStats(log QueryLog) {
	l.statsMu.Lock()
	defer l.statsMu.Unlock()

	l.stats.TotalQueries++
	l.stats.TotalDuration += log.Duration

	if log.Error != "" {
		l.stats.TotalErrors++
	}

	if log.IsSlow {
		l.stats.SlowQueries++
	}

	if log.CacheHit {
		l.stats.CacheHits++
	} else {
		l.stats.CacheMisses++
	}

	// Calculate averages
	l.stats.AverageDuration = l.stats.TotalDuration / time.Duration(l.stats.TotalQueries)

	// Per-collection stats
	if log.Collection != "" {
		if _, exists := l.stats.CollectionStats[log.Collection]; !exists {
			l.stats.CollectionStats[log.Collection] = &CollectionStats{}
		}
		cs := l.stats.CollectionStats[log.Collection]
		cs.QueryCount++
		cs.TotalDuration += log.Duration
		if log.Error != "" {
			cs.ErrorCount++
		}
		cs.AverageDuration = cs.TotalDuration / time.Duration(cs.QueryCount)
	}

	// Per-operation stats
	if log.Operation != "" {
		if _, exists := l.stats.OperationStats[log.Operation]; !exists {
			l.stats.OperationStats[log.Operation] = &OperationStats{}
		}
		os := l.stats.OperationStats[log.Operation]
		os.Count++
		os.TotalDuration += log.Duration
		if log.Error != "" {
			os.ErrorCount++
		}
		os.AverageDuration = os.TotalDuration / time.Duration(os.Count)
	}

	// Calculate rates
	elapsed := time.Since(l.stats.StartTime).Minutes()
	if elapsed > 0 {
		l.stats.QueriesPerMinute = float64(l.stats.TotalQueries) / elapsed
		l.stats.ErrorRate = float64(l.stats.TotalErrors) / float64(l.stats.TotalQueries)
	}
}

// GetStats returns current statistics
func (l *QueryLogger) GetStats() QueryStats {
	l.statsMu.RLock()
	defer l.statsMu.RUnlock()
	return l.stats
}

// GetRecentQueries returns recent queries
func (l *QueryLogger) GetRecentQueries(limit int) []QueryLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit <= 0 || limit > len(l.queries) {
		limit = len(l.queries)
	}

	// Return most recent
	start := len(l.queries) - limit
	if start < 0 {
		start = 0
	}

	result := make([]QueryLog, limit)
	copy(result, l.queries[start:])
	return result
}

// GetSlowQueries returns slow queries
func (l *QueryLogger) GetSlowQueries(limit int) []QueryLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	var slow []QueryLog
	for i := len(l.queries) - 1; i >= 0 && len(slow) < limit; i-- {
		if l.queries[i].IsSlow {
			slow = append(slow, l.queries[i])
		}
	}

	return slow
}

// GetErrorQueries returns queries with errors
func (l *QueryLogger) GetErrorQueries(limit int) []QueryLog {
	l.mu.Lock()
	defer l.mu.Unlock()

	var errors []QueryLog
	for i := len(l.queries) - 1; i >= 0 && len(errors) < limit; i-- {
		if l.queries[i].Error != "" {
			errors = append(errors, l.queries[i])
		}
	}

	return errors
}

// Clear clears all logged queries
func (l *QueryLogger) Clear() {
	l.mu.Lock()
	l.queries = make([]QueryLog, 0)
	l.mu.Unlock()

	l.statsMu.Lock()
	l.stats = QueryStats{
		CollectionStats: make(map[string]*CollectionStats),
		OperationStats:  make(map[string]*OperationStats),
		StartTime:       time.Now(),
	}
	l.statsMu.Unlock()
}

// Close closes the logger
func (l *QueryLogger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// LoggingMiddleware returns a logging context helper
type LoggingContext struct {
	logger     *QueryLogger
	startTime  time.Time
	collection string
	operation  string
	role       string
	userID     string
}

// StartQuery starts logging a query
func (l *QueryLogger) StartQuery(ctx context.Context, collection, operation string) *LoggingContext {
	return &LoggingContext{
		logger:     l,
		startTime:  time.Now(),
		collection: collection,
		operation:  operation,
	}
}

// WithRole sets the role for the query
func (lc *LoggingContext) WithRole(role string) *LoggingContext {
	lc.role = role
	return lc
}

// WithUserID sets the user ID for the query
func (lc *LoggingContext) WithUserID(userID string) *LoggingContext {
	lc.userID = userID
	return lc
}

// End ends the query logging
func (lc *LoggingContext) End(rowCount int, err error) {
	var errStr string
	if err != nil {
		errStr = err.Error()
	}

	lc.logger.Log(QueryLog{
		Collection: lc.collection,
		Operation:  lc.operation,
		Duration:   time.Since(lc.startTime),
		RowCount:   rowCount,
		Error:      errStr,
		Role:       lc.role,
		UserID:     lc.userID,
	})
}
