package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/smotra-monitoring/server/internal/config"
	"github.com/smotra-monitoring/server/internal/database"
	"gopkg.in/yaml.v3"
)

type GenerateOptions struct {
	OutputType   string // yaml or json
	DatabaseType string // postgres or sqlite
	Mode         string // development, staging, production
	OutputPath   string // output file path
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		generateCmd()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s <command> [options]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  generate    Generate a configuration file\n\n")
	fmt.Fprintf(os.Stderr, "Generate options:\n")
	fmt.Fprintf(os.Stderr, "  -t, --type        Output type (yaml or json) [default: yaml]\n")
	fmt.Fprintf(os.Stderr, "  -d, --database    Database type (postgres or sqlite) [default: sqlite]\n")
	fmt.Fprintf(os.Stderr, "  -m, --mode        Environment mode (development, staging, production) [default: development]\n")
	fmt.Fprintf(os.Stderr, "  -o, --output      Output file path [default: config.yaml or config.json]\n\n")
	fmt.Fprintf(os.Stderr, "Examples:\n")
	fmt.Fprintf(os.Stderr, "  %s generate -t yaml -d postgres -m production -o configs/prod.yaml\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s generate -t json -d sqlite -m development -o configs/dev.json\n", os.Args[0])
}

func generateCmd() {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)

	outputType := fs.String("t", "yaml", "Output type (yaml or json)")
	fs.StringVar(outputType, "type", "yaml", "Output type (yaml or json)")

	databaseType := fs.String("d", "sqlite", "Database type (postgres or sqlite)")
	fs.StringVar(databaseType, "database", "sqlite", "Database type (postgres or sqlite)")

	mode := fs.String("m", "development", "Environment mode (development, staging, production)")
	fs.StringVar(mode, "mode", "development", "Environment mode (development, staging, production)")

	outputPath := fs.String("o", "", "Output file path")
	fs.StringVar(outputPath, "output", "", "Output file path")

	if err := fs.Parse(os.Args[2:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags: %v\n", err)
		os.Exit(1)
	}

	opts := GenerateOptions{
		OutputType:   *outputType,
		DatabaseType: *databaseType,
		Mode:         *mode,
		OutputPath:   *outputPath,
	}

	// Validate options
	if opts.OutputType != "yaml" && opts.OutputType != "json" {
		fmt.Fprintf(os.Stderr, "Invalid output type: %s (must be 'yaml' or 'json')\n", opts.OutputType)
		os.Exit(1)
	}

	if opts.DatabaseType != "postgres" && opts.DatabaseType != "sqlite" {
		fmt.Fprintf(os.Stderr, "Invalid database type: %s (must be 'postgres' or 'sqlite')\n", opts.DatabaseType)
		os.Exit(1)
	}

	validModes := map[string]bool{"development": true, "staging": true, "production": true}
	if !validModes[opts.Mode] {
		fmt.Fprintf(os.Stderr, "Invalid mode: %s (must be 'development', 'staging', or 'production')\n", opts.Mode)
		os.Exit(1)
	}

	// Set default output path if not provided
	if opts.OutputPath == "" {
		switch opts.OutputType {
		case "yaml":
			opts.OutputPath = "config.yaml"
		case "json":
			opts.OutputPath = "config.json"
		default:
			fmt.Fprintf(os.Stderr, "Cannot determine default output path for type: %s\n", opts.OutputType)
			os.Exit(1)
		}
	}

	if err := generateConfig(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration file generated successfully: %s\n", opts.OutputPath)
}

func generateConfig(opts GenerateOptions) error {
	// Start with default config
	cfg := config.Default()

	// Update environment mode
	cfg.Server.Environment = opts.Mode

	// Configure database based on type
	cfg.DatabaseType = opts.DatabaseType

	if opts.DatabaseType == "postgres" {
		pgCfg := database.DefaultPostgresConfig()
		cfg.PostgresConfig = &pgCfg
		cfg.SQLiteConfig = nil

		// Production mode adjustments for postgres
		if opts.Mode == "production" {
			cfg.PostgresConfig.SSLMode = "require"
			cfg.PostgresConfig.Password = ""
			cfg.PostgresConfig.MaxOpenConns = 100
			cfg.PostgresConfig.MaxIdleConns = 20
		}
	} else {
		sqliteCfg := database.DefaultSQLiteConfig()
		cfg.SQLiteConfig = &sqliteCfg
		cfg.PostgresConfig = nil
	}

	// Mode-specific adjustments
	switch opts.Mode {
	case "development":
		cfg.Logging.Level = "debug"
		cfg.Auth.JWTSecret = "development-secret-change-in-production"
	case "staging":
		cfg.Logging.Level = "info"
		cfg.Auth.JWTSecret = "staging-secret-change-in-production"
	case "production":
		cfg.Logging.Level = "warn"
		cfg.Auth.JWTSecret = "" // Must be set via environment or config file
	}

	// Marshal config based on output type
	var data []byte
	var err error

	if opts.OutputType == "yaml" {
		data, err = yaml.Marshal(cfg)
	} else {
		data, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(opts.OutputPath)
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write to file
	if err := os.WriteFile(opts.OutputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
