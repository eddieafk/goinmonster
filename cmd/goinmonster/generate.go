package goinmonster

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the goinmonster configuration
type Config struct {
	Schema    []string                  `yaml:"schema"`
	Output    OutputConfig              `yaml:"output"`
	Database  DatabaseConfig            `yaml:"database"`
	Models    map[string]string         `yaml:"models"`
	Fields    map[string]string         `yaml:"fields"`
	Relations map[string]RelationConfig `yaml:"relations"`
	Scalars   map[string]ScalarConfig   `yaml:"scalars"`
	Resolver  ResolverConfig            `yaml:"resolver"`
}

type OutputConfig struct {
	Dir       string `yaml:"dir"`
	Package   string `yaml:"package"`
	Filename  string `yaml:"filename"`
	Resolvers string `yaml:"resolvers"`
	Server    string `yaml:"server"`
}

type DatabaseConfig struct {
	Dialect string `yaml:"dialect"`
}

type RelationConfig struct {
	Type       string `yaml:"type"`
	Table      string `yaml:"table"`
	ForeignKey string `yaml:"foreignKey"`
	References string `yaml:"references"`
	JoinType   string `yaml:"joinType"`
}

type ScalarConfig struct {
	GoType    string `yaml:"go_type"`
	Marshaler string `yaml:"marshaler"`
}

type ResolverConfig struct {
	GenerateStubs bool   `yaml:"generate_stubs"`
	Layout        string `yaml:"layout"`
}

func RunGenerate() error {
	configPath := "goinmonster.yaml"

	// Check for --config flag
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}

	// Load config
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find schema files
	schemaFiles, err := findSchemaFiles(config.Schema)
	if err != nil {
		return fmt.Errorf("failed to find schema files: %w", err)
	}

	if len(schemaFiles) == 0 {
		return fmt.Errorf("no schema files found matching patterns: %v", config.Schema)
	}

	fmt.Printf("Found %d schema file(s):\n", len(schemaFiles))
	for _, f := range schemaFiles {
		fmt.Printf("  - %s\n", f)
	}

	// Read and combine schema content
	var schemaContent strings.Builder
	for _, file := range schemaFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		schemaContent.Write(content)
		schemaContent.WriteString("\n")
	}

	// Parse schema
	schema, err := parseSchema(schemaContent.String())
	if err != nil {
		return fmt.Errorf("failed to parse schema: %w", err)
	}

	// Analyze schema for generation
	analysis := analyzeSchema(schema, config)

	// Create output directory
	if err := os.MkdirAll(config.Output.Dir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate code
	generatedPath := filepath.Join(config.Output.Dir, config.Output.Filename)
	if err := generateCode(generatedPath, config, analysis); err != nil {
		return fmt.Errorf("failed to generate code: %w", err)
	}
	fmt.Printf("✓ Generated %s\n", generatedPath)

	// Generate resolver stubs if needed
	if config.Resolver.GenerateStubs {
		resolversPath := filepath.Join(config.Output.Dir, config.Output.Resolvers)
		if _, err := os.Stat(resolversPath); os.IsNotExist(err) {
			if err := generateResolvers(resolversPath, config, analysis); err != nil {
				return fmt.Errorf("failed to generate resolvers: %w", err)
			}
			fmt.Printf("✓ Generated %s\n", resolversPath)
		} else {
			fmt.Printf("• Skipped %s (already exists)\n", resolversPath)
		}
	}

	// Generate server.go if it doesn't exist
	serverPath := config.Output.Server
	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		if err := generateServer(serverPath, config, analysis, schemaContent.String()); err != nil {
			return fmt.Errorf("failed to generate server: %w", err)
		}
		fmt.Printf("✓ Generated %s\n", serverPath)
	} else {
		fmt.Printf("• Skipped %s (already exists)\n", serverPath)
	}

	fmt.Println()
	fmt.Println("Generation complete!")
	return nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Apply defaults
	if config.Output.Dir == "" {
		config.Output.Dir = "graph"
	}
	if config.Output.Package == "" {
		config.Output.Package = "graph"
	}
	if config.Output.Filename == "" {
		config.Output.Filename = "generated.go"
	}
	if config.Output.Resolvers == "" {
		config.Output.Resolvers = "resolvers.go"
	}
	if config.Output.Server == "" {
		config.Output.Server = "server.go"
	}
	if config.Database.Dialect == "" {
		config.Database.Dialect = "postgresql"
	}
	if config.Models == nil {
		config.Models = make(map[string]string)
	}
	if config.Fields == nil {
		config.Fields = make(map[string]string)
	}
	if config.Relations == nil {
		config.Relations = make(map[string]RelationConfig)
	}
	if config.Scalars == nil {
		config.Scalars = make(map[string]ScalarConfig)
	}

	return &config, nil
}

func findSchemaFiles(patterns []string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}

		for _, match := range matches {
			if !seen[match] {
				seen[match] = true
				files = append(files, match)
			}
		}
	}

	return files, nil
}
