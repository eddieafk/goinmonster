package graph

import (
	"context"
	"sync"
	"time"
)

// Context keys
type contextKey string

const (
	operationCtxKey contextKey = "goinmonster:operation"
	resolveInfoKey  contextKey = "goinmonster:resolveinfo"
	requestCtxKey   contextKey = "goinmonster:request"
	extensionsKey   contextKey = "goinmonster:extensions"
	errorsKey       contextKey = "goinmonster:errors"
	dataLoadersKey  contextKey = "goinmonster:dataloaders"
)

// RequestContext holds request-scoped data
type RequestContext struct {
	mu sync.RWMutex

	// Request identification
	RequestID string
	StartTime time.Time

	// GraphQL operation
	Query         string
	OperationName string
	Variables     map[string]interface{}

	// Parsed operation
	Operation *OperationContext

	// Response data
	Data   interface{}
	Errors []*Error

	// Extensions data
	Extensions map[string]interface{}

	// Custom data storage
	values map[string]interface{}
}

// Error represents a GraphQL error
type Error struct {
	Message    string                 `json:"message"`
	Locations  []Location             `json:"locations,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// Location represents a location in a GraphQL document
type Location struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// Error implements the error interface
func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new GraphQL error
func NewError(message string, path []interface{}) *Error {
	return &Error{
		Message: message,
		Path:    path,
	}
}

// NewErrorWithLocation creates an error with source location
func NewErrorWithLocation(message string, line, column int) *Error {
	return &Error{
		Message: message,
		Locations: []Location{
			{Line: line, Column: column},
		},
	}
}

// NewRequestContext creates a new request context
func NewRequestContext() *RequestContext {
	return &RequestContext{
		StartTime:  time.Now(),
		Variables:  make(map[string]interface{}),
		Errors:     make([]*Error, 0),
		Extensions: make(map[string]interface{}),
		values:     make(map[string]interface{}),
	}
}

// WithRequestContext adds request context to a context
func WithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, requestCtxKey, rc)
}

// GetRequestContext retrieves request context from a context
func GetRequestContext(ctx context.Context) *RequestContext {
	if rc, ok := ctx.Value(requestCtxKey).(*RequestContext); ok {
		return rc
	}
	return nil
}

// WithOperationContext adds operation context to a context
func WithOperationContext(ctx context.Context, oc *OperationContext) context.Context {
	return context.WithValue(ctx, operationCtxKey, oc)
}

// GetOperationContext retrieves operation context from a context
func GetOperationContext(ctx context.Context) *OperationContext {
	if oc, ok := ctx.Value(operationCtxKey).(*OperationContext); ok {
		return oc
	}
	return nil
}

// WithResolveInfo adds resolve info to a context
func WithResolveInfo(ctx context.Context, info *ResolveInfo) context.Context {
	return context.WithValue(ctx, resolveInfoKey, info)
}

// GetResolveInfo retrieves resolve info from a context
func GetResolveInfo(ctx context.Context) *ResolveInfo {
	if info, ok := ctx.Value(resolveInfoKey).(*ResolveInfo); ok {
		return info
	}
	return nil
}

// Set stores a value in the request context
func (rc *RequestContext) Set(key string, value interface{}) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.values[key] = value
}

// Get retrieves a value from the request context
func (rc *RequestContext) Get(key string) (interface{}, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	v, ok := rc.values[key]
	return v, ok
}

// GetString retrieves a string value
func (rc *RequestContext) GetString(key string) string {
	if v, ok := rc.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt retrieves an int value
func (rc *RequestContext) GetInt(key string) int {
	if v, ok := rc.Get(key); ok {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

// AddError adds an error to the request context
func (rc *RequestContext) AddError(err *Error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Errors = append(rc.Errors, err)
}

// AddErrorMessage adds an error message to the request context
func (rc *RequestContext) AddErrorMessage(message string) {
	rc.AddError(&Error{Message: message})
}

// HasErrors returns true if there are errors
func (rc *RequestContext) HasErrors() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.Errors) > 0
}

// SetExtension sets an extension value
func (rc *RequestContext) SetExtension(key string, value interface{}) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Extensions[key] = value
}

// GetExtension retrieves an extension value
func (rc *RequestContext) GetExtension(key string) (interface{}, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	v, ok := rc.Extensions[key]
	return v, ok
}

// Duration returns the elapsed time since request start
func (rc *RequestContext) Duration() time.Duration {
	return time.Since(rc.StartTime)
}

// DataLoaderRegistry holds data loaders for a request
type DataLoaderRegistry struct {
	mu      sync.RWMutex
	loaders map[string]*DataLoader
}

// NewDataLoaderRegistry creates a new registry
func NewDataLoaderRegistry() *DataLoaderRegistry {
	return &DataLoaderRegistry{
		loaders: make(map[string]*DataLoader),
	}
}

// Register adds a data loader to the registry
func (r *DataLoaderRegistry) Register(name string, loader *DataLoader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loaders[name] = loader
}

// Get retrieves a data loader by name
func (r *DataLoaderRegistry) Get(name string) (*DataLoader, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	loader, ok := r.loaders[name]
	return loader, ok
}

// WithDataLoaders adds data loaders to a context
func WithDataLoaders(ctx context.Context, registry *DataLoaderRegistry) context.Context {
	return context.WithValue(ctx, dataLoadersKey, registry)
}

// GetDataLoaders retrieves data loaders from a context
func GetDataLoaders(ctx context.Context) *DataLoaderRegistry {
	if r, ok := ctx.Value(dataLoadersKey).(*DataLoaderRegistry); ok {
		return r
	}
	return nil
}

// GetDataLoader retrieves a specific data loader from context
func GetDataLoader(ctx context.Context, name string) (*DataLoader, bool) {
	if r := GetDataLoaders(ctx); r != nil {
		return r.Get(name)
	}
	return nil, false
}

// Response represents a GraphQL response
type Response struct {
	Data       interface{}            `json:"data,omitempty"`
	Errors     []*Error               `json:"errors,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// HasData returns true if response has data
func (r *Response) HasData() bool {
	return r.Data != nil
}

// HasErrors returns true if response has errors
func (r *Response) HasErrors() bool {
	return len(r.Errors) > 0
}

// NewResponse creates a new response from request context
func NewResponse(rc *RequestContext) *Response {
	resp := &Response{
		Data:   rc.Data,
		Errors: rc.Errors,
	}

	if len(rc.Extensions) > 0 {
		resp.Extensions = rc.Extensions
	}

	return resp
}

// FieldContext holds context for a specific field being resolved
type FieldContext struct {
	Object     interface{}            // The parent object
	Field      *SelectedField         // The field being resolved
	Args       map[string]interface{} // Resolved arguments
	Info       *ResolveInfo           // Full resolution info
	IsMethod   bool                   // Whether this is a method resolver
	IsResolver bool                   // Whether this has a custom resolver
	Child      func(name string) (*FieldContext, error)
}

// NewFieldContext creates a new field context
func NewFieldContext(parent interface{}, field *SelectedField, info *ResolveInfo) *FieldContext {
	return &FieldContext{
		Object: parent,
		Field:  field,
		Args:   field.Arguments,
		Info:   info,
	}
}

// ArgumentValue retrieves an argument value with type conversion
func (fc *FieldContext) ArgumentValue(name string) interface{} {
	if fc.Args == nil {
		return nil
	}
	return fc.Args[name]
}

// ArgumentString retrieves a string argument
func (fc *FieldContext) ArgumentString(name string) string {
	if v := fc.ArgumentValue(name); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ArgumentInt retrieves an int argument
func (fc *FieldContext) ArgumentInt(name string) int {
	if v := fc.ArgumentValue(name); v != nil {
		switch val := v.(type) {
		case int:
			return val
		case int64:
			return int(val)
		case float64:
			return int(val)
		}
	}
	return 0
}

// ArgumentBool retrieves a bool argument
func (fc *FieldContext) ArgumentBool(name string) bool {
	if v := fc.ArgumentValue(name); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ArgumentSlice retrieves a slice argument
func (fc *FieldContext) ArgumentSlice(name string) []interface{} {
	if v := fc.ArgumentValue(name); v != nil {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	return nil
}

// ArgumentMap retrieves a map argument
func (fc *FieldContext) ArgumentMap(name string) map[string]interface{} {
	if v := fc.ArgumentValue(name); v != nil {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}
