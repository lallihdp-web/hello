package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hasura/ndc-sdk-go/connector"
	"github.com/your-org/ndc-clickhouse-go/analytics"
	"github.com/your-org/ndc-clickhouse-go/cache"
	chconnector "github.com/your-org/ndc-clickhouse-go/connector"
	"github.com/your-org/ndc-clickhouse-go/config"
	"github.com/your-org/ndc-clickhouse-go/console"
	"github.com/your-org/ndc-clickhouse-go/middleware"
	"github.com/your-org/ndc-clickhouse-go/subscription"
)

// Version information (set at build time)
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func main() {
	// Check for CLI commands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "introspect":
			runIntrospect()
			return
		case "validate":
			runValidate()
			return
		case "print-schema":
			runPrintSchema()
			return
		case "console":
			runConsole()
			return
		case "version":
			fmt.Printf("ndc-clickhouse %s (built: %s)\n", Version, BuildTime)
			return
		case "help", "--help", "-h":
			printHelp()
			return
		}
	}

	// Start the NDC connector server
	if err := connector.Start[config.Configuration, chconnector.State](
		chconnector.NewConnector(),
		connector.WithMetricsPrefix("ndc_clickhouse"),
		connector.WithDefaultServiceName("ndc-clickhouse"),
		connector.WithVersion(Version),
	); err != nil {
		log.Fatalf("Failed to start connector: %v", err)
	}
}

func runConsole() {
	// Parse flags
	configDir := "./config"
	consolePort := "3000"
	connectorURL := "http://localhost:8080"

	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case (arg == "--config" || arg == "-c") && i+1 < len(os.Args):
			i++
			configDir = os.Args[i]
		case arg == "--port" && i+1 < len(os.Args):
			i++
			consolePort = os.Args[i]
		case arg == "--connector-url" && i+1 < len(os.Args):
			i++
			connectorURL = os.Args[i]
		}
	}

	// Use environment variables if not set
	if port := os.Getenv("CONSOLE_PORT"); port != "" && consolePort == "3000" {
		consolePort = port
	}
	if url := os.Getenv("CONNECTOR_URL"); url != "" && connectorURL == "http://localhost:8080" {
		connectorURL = url
	}

	// Initialize components
	queryCache := cache.NewQueryCache(cache.DefaultCacheConfig())
	queryLogger := analytics.NewQueryLogger(analytics.DefaultLoggerConfig())
	rateLimiter := middleware.NewRateLimiter(middleware.DefaultRateLimitConfig())
	authenticator := middleware.NewAuthenticator(middleware.DefaultAuthConfig())
	subManager := subscription.NewManager(nil, 5*time.Second) // Will be configured with actual client

	// Create console server
	consoleServer := console.NewServer(&console.Config{
		ConfigDir:    configDir,
		ConnectorURL: connectorURL,
	})

	// Create HTTP handler with middleware chain
	handler := authenticator.Middleware(
		rateLimiter.Middleware(
			consoleServer.Handler(),
		),
	)

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + consolePort,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start subscription manager
	subManager.Start()
	defer subManager.Stop()

	// Start cleanup routines
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			queryCache.Cleanup()
			rateLimiter.Cleanup(10 * time.Minute)
		}
	}()

	// Start server in goroutine
	go func() {
		log.Printf("Starting console server on http://localhost:%s", consolePort)
		log.Printf("Connector URL: %s", connectorURL)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Console server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down console server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Print stats
	stats := queryLogger.GetStats()
	log.Printf("Total queries: %d, Errors: %d", stats.TotalQueries, stats.TotalErrors)

	log.Println("Console server stopped")
}

func runIntrospect() {
	cmd := &IntrospectCmd{}

	// Parse flags
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--url" && i+1 < len(os.Args):
			i++
			cmd.URL = os.Args[i]
		case arg == "--database" && i+1 < len(os.Args):
			i++
			cmd.Database = os.Args[i]
		case arg == "--username" && i+1 < len(os.Args):
			i++
			cmd.Username = os.Args[i]
		case arg == "--password" && i+1 < len(os.Args):
			i++
			cmd.Password = os.Args[i]
		case arg == "--output" || arg == "-o" && i+1 < len(os.Args):
			i++
			cmd.Output = os.Args[i]
		case arg == "--auto-rel":
			cmd.AutoRel = true
		case arg == "--no-auto-rel":
			cmd.AutoRel = false
		}
	}

	// Set defaults
	if cmd.Database == "" {
		cmd.Database = os.Getenv("CLICKHOUSE_DATABASE")
		if cmd.Database == "" {
			cmd.Database = "default"
		}
	}
	if cmd.URL == "" {
		cmd.URL = os.Getenv("CLICKHOUSE_URL")
		if cmd.URL == "" {
			cmd.URL = "clickhouse://localhost:9000"
		}
	}
	if cmd.Username == "" {
		cmd.Username = os.Getenv("CLICKHOUSE_USERNAME")
		if cmd.Username == "" {
			cmd.Username = "default"
		}
	}
	if cmd.Password == "" {
		cmd.Password = os.Getenv("CLICKHOUSE_PASSWORD")
	}
	if cmd.Output == "" {
		cmd.Output = "./config"
	}
	if !cmd.AutoRel {
		cmd.AutoRel = true
	}

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func runValidate() {
	cmd := &ValidateCmd{Config: "./config"}

	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if (arg == "--config" || arg == "-c") && i+1 < len(os.Args) {
			i++
			cmd.Config = os.Args[i]
		}
	}

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func runPrintSchema() {
	cmd := &PrintSchemaCmd{Config: "./config", Format: "graphql"}

	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case (arg == "--config" || arg == "-c") && i+1 < len(os.Args):
			i++
			cmd.Config = os.Args[i]
		case arg == "--format" && i+1 < len(os.Args):
			i++
			cmd.Format = os.Args[i]
		}
	}

	if err := cmd.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func printHelp() {
	help := `
ndc-clickhouse - ClickHouse Native Data Connector for Hasura

USAGE:
    ndc-clickhouse <command> [options]

COMMANDS:
    serve           Start the NDC connector server (default)
    console         Start the web-based admin console
    introspect      Introspect ClickHouse database and generate configuration
    validate        Validate the connector configuration
    print-schema    Print the GraphQL schema
    version         Print version information
    help            Show this help message

CONSOLE OPTIONS:
    --port          Console server port (env: CONSOLE_PORT, default: 3000)
    --config, -c    Configuration directory (default: ./config)
    --connector-url NDC connector URL (env: CONNECTOR_URL, default: http://localhost:8080)

INTROSPECT OPTIONS:
    --url           ClickHouse connection URL (env: CLICKHOUSE_URL)
    --database      Database name (env: CLICKHOUSE_DATABASE, default: default)
    --username      Username (env: CLICKHOUSE_USERNAME, default: default)
    --password      Password (env: CLICKHOUSE_PASSWORD)
    --output, -o    Output directory (default: ./config)
    --auto-rel      Auto-detect relationships (default: true)
    --no-auto-rel   Disable relationship auto-detection

VALIDATE OPTIONS:
    --config, -c    Configuration directory (default: ./config)

PRINT-SCHEMA OPTIONS:
    --config, -c    Configuration directory (default: ./config)
    --format        Output format: graphql, json (default: graphql)

EXAMPLES:
    # Start the connector server
    ndc-clickhouse serve --configuration ./config

    # Start the admin console
    ndc-clickhouse console --port 3000 --connector-url http://localhost:8080

    # Introspect and generate configuration
    ndc-clickhouse introspect --url clickhouse://localhost:9000 --database mydb

    # Validate configuration
    ndc-clickhouse validate --config ./config

    # Print GraphQL schema
    ndc-clickhouse print-schema --config ./config

ENVIRONMENT VARIABLES:
    CLICKHOUSE_URL          Connection URL
    CLICKHOUSE_DATABASE     Database name
    CLICKHOUSE_USERNAME     Username
    CLICKHOUSE_PASSWORD     Password
    HASURA_CONNECTOR_PORT   Server port (default: 8080)
    HASURA_LOG_LEVEL        Log level (debug, info, warn, error)
    CONSOLE_PORT            Console server port (default: 3000)
    CONNECTOR_URL           NDC connector URL

For more information, visit: https://github.com/your-org/ndc-clickhouse-go
`
	fmt.Println(help)
}
