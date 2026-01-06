package goinmonster

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfigContent = `# goinmonster configuration file
# See documentation for more options

# GraphQL schema files (supports glob patterns)
schema:
  - "*.graphqls"
  - "*.graphql"
  - "graph/*.graphqls"
  - "graph/*.graphql"

# Output configuration
output:
  # Directory for generated files
  dir: "graph"
  # Package name for generated code
  package: "graph"
  # Generated file name
  filename: "generated.go"
  # Resolver file name (stub resolvers)
  resolvers: "resolvers.go"
  # Server file name (main entry point)
  server: "server.go"

# Database configuration
database:
  # SQL dialect: postgresql, mysql, sqlite
  dialect: "postgresql"

# Model configuration
models:
  # Map GraphQL types to database tables
  # Format: TypeName: table_name
  # Example:
  # User: users
  # Post: posts
  # Profile: profiles

# Field mappings (for non-standard column names)
# Format: TypeName.fieldName: column_name
# Example:
# Post.authorId: author_id
# User.createdAt: created_at
fields: {}

# Relation configurations
# Format:
#   TypeName.fieldName:
#     type: hasMany|hasOne|belongsTo
#     table: target_table
#     foreignKey: foreign_key_column
#     references: referenced_column
#     joinType: left|inner|right
relations: {}

# Custom scalar type mappings
scalars:
  DateTime:
    go_type: "time.Time"
    marshaler: "DateTimeMarshaler"
  JSON:
    go_type: "map[string]interface{}"
    marshaler: "JSONMarshaler"
  ID:
    go_type: "string"
  
# Resolver generation options
resolver:
  # Generate resolver stubs for missing resolvers
  generate_stubs: true
  # Layout: single-file or follow-schema
  layout: "single-file"
`

func RunInit() error {
	configPath := "goinmonster.yaml"

	// Check for --config flag
	for i, arg := range os.Args {
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config file %s already exists. Use --force to overwrite", configPath)
	}

	// Check for --force flag
	force := false
	for _, arg := range os.Args {
		if arg == "--force" || arg == "-f" {
			force = true
			break
		}
	}

	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("config file %s already exists", configPath)
		}
	}

	// Create config file
	if err := os.WriteFile(configPath, []byte(defaultConfigContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✓ Created %s\n", configPath)

	// Create default schema file if it doesn't exist
	schemaPath := "schema.graphqls"
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		if err := os.WriteFile(schemaPath, []byte(defaultSchemaContent), 0644); err != nil {
			return fmt.Errorf("failed to write schema file: %w", err)
		}
		fmt.Printf("✓ Created %s\n", schemaPath)
	}

	// Create graph directory if it doesn't exist
	graphDir := "graph"
	if err := os.MkdirAll(graphDir, 0755); err != nil {
		return fmt.Errorf("failed to create graph directory: %w", err)
	}
	fmt.Printf("✓ Created %s/ directory\n", graphDir)

	// Create a basic model.go file
	modelPath := filepath.Join(graphDir, "model.go")
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		if err := os.WriteFile(modelPath, []byte(defaultModelContent), 0644); err != nil {
			return fmt.Errorf("failed to write model file: %w", err)
		}
		fmt.Printf("✓ Created %s\n", modelPath)
	}

	fmt.Println()
	fmt.Println("Project initialized! Next steps:")
	fmt.Println("  1. Edit schema.graphqls with your GraphQL schema")
	fmt.Println("  2. Configure goinmonster.yaml with your database mappings")
	fmt.Println("  3. Run 'goinmonster generate' to generate code")
	fmt.Println()

	return nil
}

const defaultSchemaContent = `# GraphQL schema
# Add your types, queries, and mutations here

# Custom scalars
scalar DateTime
scalar JSON

# Directives for SQL mapping
directive @sql(
  column: String
  table: String
  relation: String
) on FIELD_DEFINITION

# Example types - replace with your own

type Query {
  users(limit: Int, offset: Int): [User!]!
  user(id: ID!): User
}

type Mutation {
  createUser(input: CreateUserInput!): User!
  updateUser(id: ID!, input: UpdateUserInput!): User!
  deleteUser(id: ID!): Boolean!
}

type User {
  id: ID!
  name: String!
  email: String!
  createdAt: DateTime!
  updatedAt: DateTime
}

input CreateUserInput {
  name: String!
  email: String!
}

input UpdateUserInput {
  name: String
  email: String
}
`

const defaultModelContent = `package graph

import "time"

// Add your custom model types here
// These will be used by the generated resolvers

// Example model - replace with your own
type User struct {
	ID        string     ` + "`json:\"id\"`" + `
	Name      string     ` + "`json:\"name\"`" + `
	Email     string     ` + "`json:\"email\"`" + `
	CreatedAt time.Time  ` + "`json:\"createdAt\"`" + `
	UpdatedAt *time.Time ` + "`json:\"updatedAt,omitempty\"`" + `
}
`
