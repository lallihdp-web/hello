// Package graphql provides a standalone GraphQL server for ClickHouse
// without requiring Hasura or any external GraphQL engine.
package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/your-org/ndc-clickhouse-go/clickhouse"
)

// ServerConfig holds configuration for the GraphQL server
type ServerConfig struct {
	// Port to listen on
	Port int

	// Enable GraphQL Playground
	EnablePlayground bool

	// Enable introspection
	EnableIntrospection bool

	// Request timeout
	Timeout time.Duration

	// CORS settings
	AllowedOrigins []string

	// Max query depth (0 = unlimited)
	MaxQueryDepth int

	// Enable query logging
	EnableQueryLogging bool

	// Slow query threshold for logging
	SlowQueryThreshold time.Duration
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:               4000,
		EnablePlayground:   true,
		EnableIntrospection: true,
		Timeout:            30 * time.Second,
		AllowedOrigins:     []string{"*"},
		MaxQueryDepth:      10,
		EnableQueryLogging: true,
		SlowQueryThreshold: 500 * time.Millisecond,
	}
}

// Server is a standalone GraphQL server for ClickHouse
type Server struct {
	config     *ServerConfig
	client     *clickhouse.Client
	schema     *Schema
	httpServer *http.Server
	mu         sync.RWMutex
}

// NewServer creates a new GraphQL server
func NewServer(client *clickhouse.Client, config *ServerConfig) (*Server, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	s := &Server{
		config: config,
		client: client,
	}

	// Build schema from ClickHouse
	if err := s.buildSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to build schema: %w", err)
	}

	return s, nil
}

// buildSchema introspects ClickHouse and builds GraphQL schema
func (s *Server) buildSchema(ctx context.Context) error {
	tables, err := s.client.GetTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to get tables: %w", err)
	}

	allColumns, err := s.client.GetAllColumns(ctx)
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.schema = NewSchema()

	for _, table := range tables {
		columns, ok := allColumns[table.Name]
		if !ok {
			continue
		}

		// Create GraphQL type for table
		s.schema.AddType(table.Name, columns)
	}

	return nil
}

// RefreshSchema rebuilds the schema from ClickHouse
func (s *Server) RefreshSchema(ctx context.Context) error {
	return s.buildSchema(ctx)
}

// Start starts the GraphQL server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// GraphQL endpoint
	mux.HandleFunc("/graphql", s.corsMiddleware(s.handleGraphQL))

	// Health check
	mux.HandleFunc("/health", s.handleHealth)

	// Schema endpoint
	mux.HandleFunc("/schema", s.handleSchema)

	// Playground (if enabled)
	if s.config.EnablePlayground {
		mux.HandleFunc("/", s.handlePlayground)
	}

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      mux,
		ReadTimeout:  s.config.Timeout,
		WriteTimeout: s.config.Timeout,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("GraphQL server starting on http://localhost:%d", s.config.Port)
	log.Printf("GraphQL Playground: http://localhost:%d/", s.config.Port)

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := false

		for _, o := range s.config.AllowedOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Hasura-Role")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// GraphQLRequest represents a GraphQL request
type GraphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// GraphQLResponse represents a GraphQL response
type GraphQLResponse struct {
	Data   interface{}      `json:"data,omitempty"`
	Errors []GraphQLError   `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message    string                 `json:"message"`
	Locations  []ErrorLocation        `json:"locations,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ErrorLocation represents error location in query
type ErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// handleGraphQL handles GraphQL requests
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" && r.Method != "GET" {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req GraphQLRequest

	if r.Method == "POST" {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "Invalid JSON")
			return
		}
	} else {
		req.Query = r.URL.Query().Get("query")
		req.OperationName = r.URL.Query().Get("operationName")
		if vars := r.URL.Query().Get("variables"); vars != "" {
			json.Unmarshal([]byte(vars), &req.Variables)
		}
	}

	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Query is required")
		return
	}

	start := time.Now()

	// Execute query
	ctx, cancel := context.WithTimeout(r.Context(), s.config.Timeout)
	defer cancel()

	result, err := s.executeQuery(ctx, req)

	duration := time.Since(start)

	// Log slow queries
	if s.config.EnableQueryLogging && duration > s.config.SlowQueryThreshold {
		log.Printf("[SLOW QUERY] %s took %v", req.OperationName, duration)
	}

	if err != nil {
		response := GraphQLResponse{
			Errors: []GraphQLError{{Message: err.Error()}},
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	json.NewEncoder(w).Encode(result)
}

// executeQuery executes a GraphQL query
func (s *Server) executeQuery(ctx context.Context, req GraphQLRequest) (*GraphQLResponse, error) {
	s.mu.RLock()
	schema := s.schema
	s.mu.RUnlock()

	// Parse and execute query
	executor := NewExecutor(s.client, schema)
	return executor.Execute(ctx, req)
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check ClickHouse connection
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := s.client.Query(ctx, "SELECT 1")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
	})
}

// handleSchema returns the GraphQL schema
func (s *Server) handleSchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	s.mu.RLock()
	defer s.mu.RUnlock()

	w.Write([]byte(s.schema.SDL()))
}

// handlePlayground serves GraphQL Playground
func (s *Server) handlePlayground(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(playgroundHTML))
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(GraphQLResponse{
		Errors: []GraphQLError{{Message: message}},
	})
}

// GetSchema returns the current schema
func (s *Server) GetSchema() *Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.schema
}

const playgroundHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>GraphQL Playground - ClickHouse</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
  <div id="root">
    <style>
      body { background-color: rgb(23, 42, 58); font-family: Open Sans, sans-serif; height: 90vh; }
      #root { height: 100%; width: 100%; display: flex; align-items: center; justify-content: center; }
      .loading { font-size: 32px; font-weight: 200; color: rgba(255, 255, 255, .6); margin-left: 28px; }
      img { width: 78px; height: 78px; }
      .title { font-weight: 400; }
    </style>
    <img src='https://cdn.jsdelivr.net/npm/graphql-playground-react/build/logo.png' alt=''>
    <div class="loading">Loading <span class="title">ClickHouse GraphQL</span></div>
  </div>
  <script>
    window.addEventListener('load', function() {
      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: '/graphql',
        settings: {
          'editor.theme': 'dark',
          'editor.fontSize': 14,
          'tracing.hideTracingResponse': true,
        },
        tabs: [{
          endpoint: '/graphql',
          query: '# Welcome to ClickHouse GraphQL!\n#\n# Example query:\n# query {\n#   <table_name>(limit: 10) {\n#     <column1>\n#     <column2>\n#   }\n# }\n',
        }],
      });
    });
  </script>
</body>
</html>`
