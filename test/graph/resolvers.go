package graph

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/eddieafk/goinmonster/graph"
)

// Resolver is the root resolver
type Resolver struct {
	// Add your dependencies here (e.g., database connection)
	DB *sql.DB
}

// Query returns the query resolver
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

// Mutation returns the mutation resolver
func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}

type queryResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }

// Users resolves Query.users
func (r *queryResolver) Users(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		info := graph.GetResolveInfo(ctx)
		if info == nil {
			return nil, fmt.Errorf("missing resolve info")
		}

		// Convert to SQL query
		result, err := sqlConverter.ConvertToSelect(ctx, info)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute query against your database
		rows, err := r.DB.QueryContext(ctx, result.Query, result.Params...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var users []map[string]interface{}
		for rows.Next() {
			var id, name, email string
			if err := rows.Scan(&id, &name, &email); err != nil {
				return nil, err
			}
			users = append(users, map[string]interface{}{
				"id":    id,
				"name":  name,
				"email": email,
			})
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return users, nil

	}
}

// User resolves Query.user
func (r *queryResolver) User(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		info := graph.GetResolveInfo(ctx)
		if info == nil {
			return nil, fmt.Errorf("missing resolve info")
		}

		// Convert to SQL query
		result, err := sqlConverter.ConvertToSelect(ctx, info)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute query against your database
		// rows, err := db.QueryContext(ctx, result.Query, result.Params...)

		// Return mock data for now
		return map[string]interface{}{}, nil

	}
}

// __schema resolves Query.__schema
func (r *queryResolver) __schema(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		info := graph.GetResolveInfo(ctx)
		if info == nil {
			return nil, fmt.Errorf("missing resolve info")
		}

		// Convert to SQL query
		result, err := sqlConverter.ConvertToSelect(ctx, info)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute query against your database
		// rows, err := db.QueryContext(ctx, result.Query, result.Params...)

		// Return mock data for now
		return map[string]interface{}{}, nil

	}
}

// __type resolves Query.__type
func (r *queryResolver) __type(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		info := graph.GetResolveInfo(ctx)
		if info == nil {
			return nil, fmt.Errorf("missing resolve info")
		}

		// Convert to SQL query
		result, err := sqlConverter.ConvertToSelect(ctx, info)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute query against your database
		// rows, err := db.QueryContext(ctx, result.Query, result.Params...)

		// Return mock data for now
		return map[string]interface{}{}, nil

	}
}

// CreateUser resolves Mutation.createUser
func (r *mutationResolver) CreateUser(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		input, ok := args["input"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("input is required")
		}

		info := graph.GetResolveInfo(ctx)
		returning := []string{"id"}
		if info != nil && info.Selection != nil {
			returning = graph.GetFieldNames(info.Selection)
		}

		result, err := sqlConverter.ConvertToInsert(ctx, "User", input, returning)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		row := r.DB.QueryRowContext(ctx, result.Query, result.Params...)
		var id, name string
		if err := row.Scan(&id, &name); err != nil {
			return nil, err
		}

		return map[string]interface{}{
			"id":   id,
			"name": name,
		}, nil

	}
}

// UpdateUser resolves Mutation.updateUser
func (r *mutationResolver) UpdateUser(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		input, ok := args["input"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("input is required")
		}

		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("id is required")
		}

		info := graph.GetResolveInfo(ctx)
		returning := []string{"id"}
		if info != nil && info.Selection != nil {
			returning = graph.GetFieldNames(info.Selection)
		}

		where := map[string]interface{}{"id": id}
		result, err := sqlConverter.ConvertToUpdate(ctx, "User", where, input, returning)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute mutation against your database

		return map[string]interface{}{}, nil

	}
}

// DeleteUser resolves Mutation.deleteUser
func (r *mutationResolver) DeleteUser(es *graph.ExecutableSchema) graph.ResolverFunc {
	return func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		id, ok := args["id"].(string)
		if !ok {
			return nil, fmt.Errorf("id is required")
		}

		info := graph.GetResolveInfo(ctx)
		returning := []string{"id"}
		if info != nil && info.Selection != nil {
			returning = graph.GetFieldNames(info.Selection)
		}

		where := map[string]interface{}{"id": id}
		result, err := sqlConverter.ConvertToDelete(ctx, "User", where, returning)
		if err != nil {
			return nil, err
		}

		log.Printf("Generated SQL: %s", result.Query)
		log.Printf("Parameters: %v", result.Params)

		// TODO: Execute mutation against your database

		return true, nil

	}
}

// Ensure imports are used
var (
	_ = time.Now
	_ = log.Printf
)
