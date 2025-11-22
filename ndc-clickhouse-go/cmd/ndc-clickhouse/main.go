package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hasura/ndc-sdk-go/connector"
	chconnector "github.com/your-org/ndc-clickhouse-go/connector"
	"github.com/your-org/ndc-clickhouse-go/config"
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
    introspect      Introspect ClickHouse database and generate configuration
    validate        Validate the connector configuration
    print-schema    Print the GraphQL schema
    version         Print version information
    help            Show this help message

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

For more information, visit: https://github.com/your-org/ndc-clickhouse-go
`
	fmt.Println(help)
}
