package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"test/graph"

	gqlgraph "github.com/eddieafk/goinmonster/graph"
	"github.com/eddieafk/goinmonster/handler"

	_ "github.com/lib/pq"
)

func main() {
	// Load schema from embedded string or file
	schemaString := `# GraphQL schema
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

	// Initialize database connection (PostgreSQL example)
	dsn := "postgres://(username):(password)@localhost:5432/(database_name)?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	// Create the executable schema
	es, err := gqlgraph.NewExecutableSchema(schemaString)
	if err != nil {
		log.Fatalf("Failed to create schema: %v", err)
	}

	// Initialize SQL converter with mappings
	graph.InitSQLConverter(es.Schema)

	// Register custom scalar marshalers
	graph.RegisterMarshalers(es.Schema)

	// Register resolvers
	resolver := &graph.Resolver{
		// TODO: Add your dependencies here (e.g., database connection)
		// Reference to; ./graph/resolvers.Resolver{}
		DB: db,
	}
	graph.RegisterResolvers(es, resolver)

	// Create the HTTP server
	srv := handler.New(es)

	// Add transports
	srv.AddTransport(handler.NewOPTIONS())
	srv.AddTransport(handler.NewGET())
	srv.AddTransport(handler.NewPOST())
	srv.AddTransport(handler.NewMultipartForm())

	// Add extensions
	srv.Use(handler.NewTracing())
	srv.Use(handler.NewComplexityLimit(1000))
	srv.Use(handler.NewRequestLogger(func(ctx context.Context, query, opName string, vars map[string]interface{}, duration time.Duration) {
		log.Printf("[GraphQL] %s (%s) took %v", opName, truncateQuery(query), duration)
	}))
	srv.Use(handler.NewErrorLogger(func(ctx context.Context, err *gqlgraph.Error) {
		log.Printf("[GraphQL Error] %s", err.Message)
	}))

	// Set up HTTP handler with CORS
	mux := http.NewServeMux()
	mux.Handle("/graphql", corsMiddleware(srv))
	mux.Handle("/playground", corsMiddleware(srv))

	// Start server
	addr := ":8080"
	log.Printf("🚀 GraphQL server running at http://localhost%s/graphql", addr)
	log.Printf("🎮 Playground available at http://localhost%s/playground", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// corsMiddleware adds CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// truncateQuery truncates a query for logging
func truncateQuery(query string) string {
	if len(query) > 100 {
		return query[:100] + "..."
	}
	return query
}
