package graph

import (
	"context"
	"fmt"
	"reflect"
)

// ResolverFunc is the signature for field resolver functions
type ResolverFunc func(ctx context.Context, args map[string]interface{}) (interface{}, error)

// FieldResolver handles resolution for a specific field
type FieldResolver struct {
	TypeName   string
	FieldName  string
	ResolverFn ResolverFunc
	Middleware []MiddlewareFunc
}

// MiddlewareFunc wraps resolver execution
type MiddlewareFunc func(ctx context.Context, next ResolverFunc) ResolverFunc

// ResolverMap holds all resolvers organized by type and field
type ResolverMap struct {
	resolvers map[string]map[string]*FieldResolver
}

// NewResolverMap creates a new resolver map
func NewResolverMap() *ResolverMap {
	return &ResolverMap{
		resolvers: make(map[string]map[string]*FieldResolver),
	}
}

// Register adds a resolver for a specific type and field
func (rm *ResolverMap) Register(typeName, fieldName string, resolver ResolverFunc) {
	if rm.resolvers[typeName] == nil {
		rm.resolvers[typeName] = make(map[string]*FieldResolver)
	}

	rm.resolvers[typeName][fieldName] = &FieldResolver{
		TypeName:   typeName,
		FieldName:  fieldName,
		ResolverFn: resolver,
	}
}

// RegisterWithMiddleware adds a resolver with middleware
func (rm *ResolverMap) RegisterWithMiddleware(typeName, fieldName string, resolver ResolverFunc, middleware ...MiddlewareFunc) {
	if rm.resolvers[typeName] == nil {
		rm.resolvers[typeName] = make(map[string]*FieldResolver)
	}

	rm.resolvers[typeName][fieldName] = &FieldResolver{
		TypeName:   typeName,
		FieldName:  fieldName,
		ResolverFn: resolver,
		Middleware: middleware,
	}
}

// Get retrieves a resolver for a type and field
func (rm *ResolverMap) Get(typeName, fieldName string) (*FieldResolver, bool) {
	typeResolvers, ok := rm.resolvers[typeName]
	if !ok {
		return nil, false
	}

	resolver, ok := typeResolvers[fieldName]
	return resolver, ok
}

// HasType checks if any resolvers exist for a type
func (rm *ResolverMap) HasType(typeName string) bool {
	_, ok := rm.resolvers[typeName]
	return ok
}

// Resolve executes the resolver for a field with middleware chain
func (fr *FieldResolver) Resolve(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	resolver := fr.ResolveWithMiddleware()
	return resolver(ctx, args)
}

// ResolveWithMiddleware builds the middleware chain and returns the final resolver
func (fr *FieldResolver) ResolveWithMiddleware() ResolverFunc {
	resolver := fr.ResolverFn

	// Apply middleware in reverse order (last registered runs first)
	for i := len(fr.Middleware) - 1; i >= 0; i-- {
		mw := fr.Middleware[i]
		next := resolver
		resolver = func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			return mw(ctx, next)(ctx, args)
		}
	}

	return resolver
}

// RootResolver is the interface that user-defined resolvers must implement
type RootResolver interface{}

// SQLResolver provides SQL-specific resolution capabilities
type SQLResolver interface {
	// ToSQLSelect converts GraphQL query to SQL SELECT options
	ToSQLSelect(ctx context.Context, info *ResolveInfo) (interface{}, error)
}

// ResolveInfo contains information about the current resolution
type ResolveInfo struct {
	FieldName    string
	ParentType   string
	ReturnType   *TypeRef
	Arguments    map[string]interface{}
	Variables    map[string]interface{}
	Selection    *SelectionSet
	Path         []string
	RootValue    interface{}
	OperationCtx *OperationContext
}

// SelectionSet represents selected fields in a query
type SelectionSet struct {
	Fields   []*SelectedField
	Typename bool // Whether __typename was requested
}

// SelectedField represents a selected field with its arguments and nested selections
type SelectedField struct {
	Name       string
	Alias      string
	Arguments  map[string]interface{}
	Selections *SelectionSet
	Directives []*DirectiveInstance
}

// DirectiveInstance represents a directive applied to a field in a query
type DirectiveInstance struct {
	Name      string
	Arguments map[string]interface{}
}

// GetName returns the alias if set, otherwise the field name
func (sf *SelectedField) GetName() string {
	if sf.Alias != "" {
		return sf.Alias
	}
	return sf.Name
}

// HasSelection checks if a field has nested selections
func (sf *SelectedField) HasSelection() bool {
	return sf.Selections != nil && len(sf.Selections.Fields) > 0
}

// OperationContext holds context for the entire operation
type OperationContext struct {
	OperationType string // "query", "mutation", "subscription"
	OperationName string
	Variables     map[string]interface{}
	Schema        *Schema
	RootResolver  RootResolver
}

// DataLoader provides batching and caching for resolver data fetching
type DataLoader struct {
	batchFn    func(ctx context.Context, keys []interface{}) ([]interface{}, []error)
	cache      map[interface{}]interface{}
	batch      []interface{}
	batchErr   []chan loadResult
	maxBatch   int
	batchDelay int // milliseconds
}

type loadResult struct {
	data interface{}
	err  error
}

// NewDataLoader creates a new data loader with batching
func NewDataLoader(batchFn func(ctx context.Context, keys []interface{}) ([]interface{}, []error), opts ...DataLoaderOption) *DataLoader {
	dl := &DataLoader{
		batchFn:    batchFn,
		cache:      make(map[interface{}]interface{}),
		batch:      make([]interface{}, 0),
		batchErr:   make([]chan loadResult, 0),
		maxBatch:   100,
		batchDelay: 1,
	}

	for _, opt := range opts {
		opt(dl)
	}

	return dl
}

// DataLoaderOption configures a DataLoader
type DataLoaderOption func(*DataLoader)

// WithMaxBatch sets the maximum batch size
func WithMaxBatch(size int) DataLoaderOption {
	return func(dl *DataLoader) {
		dl.maxBatch = size
	}
}

// WithBatchDelay sets the delay before batching
func WithBatchDelay(ms int) DataLoaderOption {
	return func(dl *DataLoader) {
		dl.batchDelay = ms
	}
}

// Load loads a single key
func (dl *DataLoader) Load(ctx context.Context, key interface{}) (interface{}, error) {
	// Check cache first
	if cached, ok := dl.cache[key]; ok {
		return cached, nil
	}

	// Add to batch
	resultCh := make(chan loadResult, 1)
	dl.batch = append(dl.batch, key)
	dl.batchErr = append(dl.batchErr, resultCh)

	// If batch is full, execute immediately
	if len(dl.batch) >= dl.maxBatch {
		dl.executeBatch(ctx)
	}

	// Wait for result
	result := <-resultCh
	return result.data, result.err
}

// executeBatch runs the batch function
func (dl *DataLoader) executeBatch(ctx context.Context) {
	if len(dl.batch) == 0 {
		return
	}

	keys := dl.batch
	channels := dl.batchErr

	// Reset batch
	dl.batch = make([]interface{}, 0)
	dl.batchErr = make([]chan loadResult, 0)

	// Execute batch function
	results, errs := dl.batchFn(ctx, keys)

	// Send results to waiting goroutines
	for i, ch := range channels {
		var result loadResult
		if i < len(results) {
			result.data = results[i]
			// Cache the result
			dl.cache[keys[i]] = results[i]
		}
		if i < len(errs) && errs[i] != nil {
			result.err = errs[i]
		}
		ch <- result
		close(ch)
	}
}

// Clear removes an item from the cache
func (dl *DataLoader) Clear(key interface{}) {
	delete(dl.cache, key)
}

// ClearAll clears the entire cache
func (dl *DataLoader) ClearAll() {
	dl.cache = make(map[interface{}]interface{})
}

// Prime adds a value to the cache
func (dl *DataLoader) Prime(key, value interface{}) {
	dl.cache[key] = value
}

// ResolverBuilder helps build resolvers from struct methods using reflection
type ResolverBuilder struct {
	resolverMap *ResolverMap
}

// NewResolverBuilder creates a new resolver builder
func NewResolverBuilder() *ResolverBuilder {
	return &ResolverBuilder{
		resolverMap: NewResolverMap(),
	}
}

// RegisterStruct registers all exported methods from a struct as resolvers
func (rb *ResolverBuilder) RegisterStruct(typeName string, resolver interface{}) error {
	val := reflect.ValueOf(resolver)
	typ := val.Type()

	if typ.Kind() != reflect.Ptr || typ.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("resolver must be a pointer to struct")
	}

	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if !method.IsExported() {
			continue
		}

		fieldName := method.Name
		methodVal := val.Method(i)

		// Wrap method as ResolverFunc
		resolver := func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			// Build method arguments
			methodType := methodVal.Type()
			callArgs := make([]reflect.Value, 0, methodType.NumIn())

			for j := 0; j < methodType.NumIn(); j++ {
				argType := methodType.In(j)

				if argType.Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
					callArgs = append(callArgs, reflect.ValueOf(ctx))
				} else if argType.Kind() == reflect.Map {
					callArgs = append(callArgs, reflect.ValueOf(args))
				} else {
					// Try to extract from args based on type
					callArgs = append(callArgs, reflect.Zero(argType))
				}
			}

			results := methodVal.Call(callArgs)

			if len(results) == 0 {
				return nil, nil
			}

			if len(results) == 1 {
				return results[0].Interface(), nil
			}

			// Assume last result is error
			var err error
			if !results[len(results)-1].IsNil() {
				err = results[len(results)-1].Interface().(error)
			}

			return results[0].Interface(), err
		}

		rb.resolverMap.Register(typeName, fieldName, resolver)
	}

	return nil
}

// Build returns the built resolver map
func (rb *ResolverBuilder) Build() *ResolverMap {
	return rb.resolverMap
}
