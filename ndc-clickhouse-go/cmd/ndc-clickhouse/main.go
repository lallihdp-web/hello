package main

import (
	"log"

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
	if err := connector.Start[config.Configuration, chconnector.State](
		chconnector.NewConnector(),
		connector.WithMetricsPrefix("ndc_clickhouse"),
		connector.WithDefaultServiceName("ndc-clickhouse"),
		connector.WithVersion(Version),
	); err != nil {
		log.Fatalf("Failed to start connector: %v", err)
	}
}
