package console

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
	"github.com/your-org/ndc-clickhouse-go/config"
)

//go:embed static/*
var staticFiles embed.FS

//go:embed templates/*
var templateFiles embed.FS

// Console represents the admin web console
type Console struct {
	config     *config.Configuration
	client     *clickhouse.Client
	httpServer *http.Server
	templates  *template.Template
	mu         sync.RWMutex

	// Analytics
	queryStats *QueryStats
}

// QueryStats tracks query analytics
type QueryStats struct {
	mu           sync.RWMutex
	TotalQueries int64
	TotalErrors  int64
	AvgDuration  float64
	SlowQueries  []SlowQuery
	RecentErrors []QueryError
}

// SlowQuery represents a slow query record
type SlowQuery struct {
	SQL       string    `json:"sql"`
	Duration  float64   `json:"duration_ms"`
	Timestamp time.Time `json:"timestamp"`
}

// QueryError represents a query error record
type QueryError struct {
	SQL       string    `json:"sql"`
	Error     string    `json:"error"`
	Timestamp time.Time `json:"timestamp"`
}

// NewConsole creates a new admin console
func NewConsole(cfg *config.Configuration, client *clickhouse.Client) (*Console, error) {
	tmpl, err := template.ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Console{
		config:    cfg,
		client:    client,
		templates: tmpl,
		queryStats: &QueryStats{
			SlowQueries:  make([]SlowQuery, 0, 100),
			RecentErrors: make([]QueryError, 0, 100),
		},
	}, nil
}

// Start starts the console HTTP server
func (c *Console) Start(addr string) error {
	mux := http.NewServeMux()

	// Static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Pages
	mux.HandleFunc("/", c.handleIndex)
	mux.HandleFunc("/playground", c.handlePlayground)
	mux.HandleFunc("/schema", c.handleSchema)
	mux.HandleFunc("/config", c.handleConfig)
	mux.HandleFunc("/permissions", c.handlePermissions)
	mux.HandleFunc("/analytics", c.handleAnalytics)
	mux.HandleFunc("/health", c.handleHealth)

	// API endpoints
	mux.HandleFunc("/api/schema", c.apiGetSchema)
	mux.HandleFunc("/api/tables", c.apiGetTables)
	mux.HandleFunc("/api/config", c.apiConfig)
	mux.HandleFunc("/api/query", c.apiExecuteQuery)
	mux.HandleFunc("/api/stats", c.apiGetStats)
	mux.HandleFunc("/api/health", c.apiHealthCheck)

	c.httpServer = &http.Server{
		Addr:         addr,
		Handler:      corsMiddleware(loggingMiddleware(mux)),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	fmt.Printf("🎨 Console available at http://%s\n", addr)
	return c.httpServer.ListenAndServe()
}

// Stop stops the console server
func (c *Console) Stop(ctx context.Context) error {
	if c.httpServer != nil {
		return c.httpServer.Shutdown(ctx)
	}
	return nil
}

// Middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("[%s] %s %s %v\n", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

// Page Handlers
func (c *Console) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Title":       "NDC ClickHouse Console",
		"Database":    c.config.Connection.Database,
		"TableCount":  len(c.config.Tables),
		"QueryCount":  c.queryStats.TotalQueries,
		"ErrorCount":  c.queryStats.TotalErrors,
	}

	c.renderTemplate(w, "index.html", data)
}

func (c *Console) handlePlayground(w http.ResponseWriter, r *http.Request) {
	c.renderTemplate(w, "playground.html", nil)
}

func (c *Console) handleSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tables, _ := c.client.GetTables(ctx)
	allColumns, _ := c.client.GetAllColumns(ctx)

	data := map[string]interface{}{
		"Tables":     tables,
		"AllColumns": allColumns,
	}

	c.renderTemplate(w, "schema.html", data)
}

func (c *Console) handleConfig(w http.ResponseWriter, r *http.Request) {
	configJSON, _ := json.MarshalIndent(c.config, "", "  ")
	data := map[string]interface{}{
		"Config": string(configJSON),
	}
	c.renderTemplate(w, "config.html", data)
}

func (c *Console) handlePermissions(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Permissions": c.config.Permissions,
	}
	c.renderTemplate(w, "permissions.html", data)
}

func (c *Console) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	c.queryStats.mu.RLock()
	data := map[string]interface{}{
		"TotalQueries": c.queryStats.TotalQueries,
		"TotalErrors":  c.queryStats.TotalErrors,
		"AvgDuration":  c.queryStats.AvgDuration,
		"SlowQueries":  c.queryStats.SlowQueries,
		"RecentErrors": c.queryStats.RecentErrors,
	}
	c.queryStats.mu.RUnlock()

	c.renderTemplate(w, "analytics.html", data)
}

func (c *Console) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	healthy := true
	var healthErr string

	_, err := c.client.Query(ctx, "SELECT 1")
	if err != nil {
		healthy = false
		healthErr = err.Error()
	}

	data := map[string]interface{}{
		"Healthy":  healthy,
		"Error":    healthErr,
		"Database": c.config.Connection.Database,
	}
	c.renderTemplate(w, "health.html", data)
}

// API Handlers
func (c *Console) apiGetSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tables, err := c.client.GetTables(ctx)
	if err != nil {
		c.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	allColumns, err := c.client.GetAllColumns(ctx)
	if err != nil {
		c.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c.jsonResponse(w, map[string]interface{}{
		"tables":  tables,
		"columns": allColumns,
	})
}

func (c *Console) apiGetTables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tables, err := c.client.GetTables(ctx)
	if err != nil {
		c.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c.jsonResponse(w, tables)
}

func (c *Console) apiConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		c.jsonResponse(w, c.config)
	case "PUT", "POST":
		var newConfig config.Configuration
		if err := json.NewDecoder(r.Body).Decode(&newConfig); err != nil {
			c.jsonError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		c.mu.Lock()
		c.config = &newConfig
		c.mu.Unlock()

		c.jsonResponse(w, map[string]string{"status": "updated"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (c *Console) apiExecuteQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	start := time.Now()
	ctx := r.Context()

	rows, err := c.client.Query(ctx, req.SQL)
	duration := time.Since(start).Milliseconds()

	// Track stats
	c.queryStats.mu.Lock()
	c.queryStats.TotalQueries++
	if err != nil {
		c.queryStats.TotalErrors++
		c.queryStats.RecentErrors = append(c.queryStats.RecentErrors, QueryError{
			SQL:       req.SQL,
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		if len(c.queryStats.RecentErrors) > 100 {
			c.queryStats.RecentErrors = c.queryStats.RecentErrors[1:]
		}
	} else if duration > 1000 { // Slow query > 1s
		c.queryStats.SlowQueries = append(c.queryStats.SlowQueries, SlowQuery{
			SQL:       req.SQL,
			Duration:  float64(duration),
			Timestamp: time.Now(),
		})
		if len(c.queryStats.SlowQueries) > 100 {
			c.queryStats.SlowQueries = c.queryStats.SlowQueries[1:]
		}
	}
	c.queryStats.mu.Unlock()

	if err != nil {
		c.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer rows.Close()

	// Get column names
	columnTypes := rows.ColumnTypes()
	columns := make([]string, len(columnTypes))
	for i, ct := range columnTypes {
		columns[i] = ct.Name()
	}

	// Collect results
	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	c.jsonResponse(w, map[string]interface{}{
		"columns":     columns,
		"rows":        results,
		"rowCount":    len(results),
		"duration_ms": duration,
	})
}

func (c *Console) apiGetStats(w http.ResponseWriter, r *http.Request) {
	c.queryStats.mu.RLock()
	defer c.queryStats.mu.RUnlock()

	c.jsonResponse(w, c.queryStats)
}

func (c *Console) apiHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, err := c.client.Query(ctx, "SELECT 1")

	status := "healthy"
	if err != nil {
		status = "unhealthy"
	}

	c.jsonResponse(w, map[string]interface{}{
		"status":   status,
		"database": c.config.Connection.Database,
		"error":    err,
	})
}

// Helper methods
func (c *Console) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *Console) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (c *Console) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Config holds server configuration
type Config struct {
	ConfigDir    string
	ConnectorURL string
}

// Server wraps the console for standalone usage
type Server struct {
	config  *Config
	handler http.Handler
}

// NewServer creates a new console server
func NewServer(cfg *Config) *Server {
	mux := http.NewServeMux()

	// Static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Parse templates
	tmpl, _ := template.ParseFS(templateFiles, "templates/*.html")

	// Simple page handlers without database connection
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := map[string]interface{}{
			"Title":        "NDC ClickHouse Console",
			"ConnectorURL": cfg.ConnectorURL,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "index.html", data)
	})

	mux.HandleFunc("/playground", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"ConnectorURL": cfg.ConnectorURL,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "playground.html", data)
	})

	mux.HandleFunc("/schema", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"ConnectorURL": cfg.ConnectorURL,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "schema.html", data)
	})

	mux.HandleFunc("/config", func(w http.ResponseWriter, r *http.Request) {
		data := map[string]interface{}{
			"ConfigDir": cfg.ConfigDir,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "config.html", data)
	})

	mux.HandleFunc("/permissions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "permissions.html", nil)
	})

	mux.HandleFunc("/analytics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "analytics.html", nil)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.ExecuteTemplate(w, "health.html", nil)
	})

	// Proxy API calls to the connector
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"connector_url": cfg.ConnectorURL,
			"message":       "Use connector URL directly for API calls",
		})
	})

	return &Server{
		config:  cfg,
		handler: corsMiddleware(loggingMiddleware(mux)),
	}
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.handler
}
