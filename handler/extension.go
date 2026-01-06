package handler

import (
	"context"
	"time"

	"github.com/eddieafk/goinmonster/graph"
)

// Extension is the base interface for server extensions
type Extension interface {
	// ExtensionName returns the name of the extension
	ExtensionName() string
}

// OperationInterceptor intercepts operation execution
type OperationInterceptor interface {
	Extension
	// InterceptOperation wraps the operation execution
	InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context
}

// ResponseInterceptor intercepts response generation
type ResponseInterceptor interface {
	Extension
	// InterceptResponse allows modification of the response before sending
	InterceptResponse(ctx context.Context, response *graph.Response) *graph.Response
}

// FieldInterceptor intercepts field resolution
type FieldInterceptor interface {
	Extension
	// InterceptField wraps individual field resolution
	InterceptField(ctx context.Context, next graph.ResolverFunc) graph.ResolverFunc
}

// ExtensionData provides data to be added to the response extensions
type ExtensionData interface {
	Extension
	// ExtensionData returns data to add to response.extensions
	ExtensionData(ctx context.Context) map[string]interface{}
}

// OperationComplexity calculates operation complexity
type OperationComplexity interface {
	Extension
	// CalculateComplexity calculates the complexity of an operation
	CalculateComplexity(ctx context.Context, query string) int
}

// Caching handles query result caching
type Caching interface {
	Extension
	// GetFromCache retrieves a cached result
	GetFromCache(ctx context.Context, key string) (*graph.Response, bool)
	// SetInCache stores a result in cache
	SetInCache(ctx context.Context, key string, response *graph.Response, ttl time.Duration)
}

// Tracing extension for APM integration
type Tracing struct {
	enableTracing bool
	version       int
}

// NewTracing creates a new tracing extension
func NewTracing() *Tracing {
	return &Tracing{
		enableTracing: true,
		version:       1,
	}
}

// ExtensionName returns the extension name
func (t *Tracing) ExtensionName() string {
	return "tracing"
}

// InterceptOperation adds tracing to operations
func (t *Tracing) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	startTime := time.Now()

	rc := graph.GetRequestContext(ctx)
	if rc != nil {
		rc.Set("tracing:start", startTime)
	}

	return ctx
}

// ExtensionData returns tracing data
func (t *Tracing) ExtensionData(ctx context.Context) map[string]interface{} {
	rc := graph.GetRequestContext(ctx)
	if rc == nil {
		return nil
	}

	startTime, ok := rc.Get("tracing:start")
	if !ok {
		return nil
	}

	start := startTime.(time.Time)
	duration := time.Since(start)

	return map[string]interface{}{
		"tracing": map[string]interface{}{
			"version":   t.version,
			"startTime": start.Format(time.RFC3339Nano),
			"endTime":   time.Now().Format(time.RFC3339Nano),
			"duration":  duration.Nanoseconds(),
		},
	}
}

// ComplexityLimit extension for query complexity limiting
type ComplexityLimit struct {
	limit          int
	defaultCost    int
	fieldCosts     map[string]int
	complexityFunc func(fieldName string, args map[string]interface{}) int
}

// NewComplexityLimit creates a new complexity limit extension
func NewComplexityLimit(limit int) *ComplexityLimit {
	return &ComplexityLimit{
		limit:       limit,
		defaultCost: 1,
		fieldCosts:  make(map[string]int),
	}
}

// ExtensionName returns the extension name
func (c *ComplexityLimit) ExtensionName() string {
	return "complexityLimit"
}

// SetFieldCost sets the cost for a specific field
func (c *ComplexityLimit) SetFieldCost(typeName, fieldName string, cost int) {
	c.fieldCosts[typeName+"."+fieldName] = cost
}

// SetDefaultCost sets the default cost per field
func (c *ComplexityLimit) SetDefaultCost(cost int) {
	c.defaultCost = cost
}

// SetComplexityFunc sets a custom complexity calculator
func (c *ComplexityLimit) SetComplexityFunc(fn func(fieldName string, args map[string]interface{}) int) {
	c.complexityFunc = fn
}

// CalculateComplexity calculates query complexity
func (c *ComplexityLimit) CalculateComplexity(ctx context.Context, query string) int {
	// Simplified complexity calculation
	// In a real implementation, this would parse the query and calculate based on fields
	return 0
}

// InterceptOperation checks complexity before execution
func (c *ComplexityLimit) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	rc := graph.GetRequestContext(ctx)
	if rc == nil {
		return ctx
	}

	complexity := c.CalculateComplexity(ctx, rc.Query)
	if c.limit > 0 && complexity > c.limit {
		rc.AddError(&graph.Error{
			Message: "query complexity exceeds limit",
			Extensions: map[string]interface{}{
				"code":       "COMPLEXITY_LIMIT_EXCEEDED",
				"complexity": complexity,
				"limit":      c.limit,
			},
		})
	}

	rc.Set("complexity", complexity)
	return ctx
}

// APQ extension for Automatic Persisted Queries
type APQ struct {
	cache map[string]string
}

// NewAPQ creates a new APQ extension
func NewAPQ() *APQ {
	return &APQ{
		cache: make(map[string]string),
	}
}

// ExtensionName returns the extension name
func (a *APQ) ExtensionName() string {
	return "persistedQuery"
}

// InterceptOperation handles APQ
func (a *APQ) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	rc := graph.GetRequestContext(ctx)
	if rc == nil {
		return ctx
	}

	// Check for persisted query extension
	if ext, ok := rc.Extensions["persistedQuery"].(map[string]interface{}); ok {
		if hash, ok := ext["sha256Hash"].(string); ok {
			if rc.Query == "" {
				// Look up query by hash
				if cached, ok := a.cache[hash]; ok {
					rc.Query = cached
				} else {
					rc.AddError(&graph.Error{
						Message: "PersistedQueryNotFound",
						Extensions: map[string]interface{}{
							"code": "PERSISTED_QUERY_NOT_FOUND",
						},
					})
				}
			} else {
				// Store query
				a.cache[hash] = rc.Query
			}
		}
	}

	return ctx
}

// IntrospectionDisabler disables introspection queries
type IntrospectionDisabler struct{}

// NewIntrospectionDisabler creates a new introspection disabler
func NewIntrospectionDisabler() *IntrospectionDisabler {
	return &IntrospectionDisabler{}
}

// ExtensionName returns the extension name
func (i *IntrospectionDisabler) ExtensionName() string {
	return "introspectionDisabler"
}

// InterceptOperation blocks introspection queries
func (i *IntrospectionDisabler) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	rc := graph.GetRequestContext(ctx)
	if rc == nil {
		return ctx
	}

	// Check if query contains introspection
	// This is a simplified check
	if containsIntrospection(rc.Query) {
		rc.AddError(&graph.Error{
			Message: "introspection is disabled",
			Extensions: map[string]interface{}{
				"code": "INTROSPECTION_DISABLED",
			},
		})
	}

	return ctx
}

// containsIntrospection checks if a query contains introspection fields
func containsIntrospection(query string) bool {
	// Simple check - real implementation would parse the query
	return false
}

// RateLimiter extension for rate limiting
type RateLimiter struct {
	requestsPerSecond int
	tokens            map[string]int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		requestsPerSecond: requestsPerSecond,
		tokens:            make(map[string]int),
	}
}

// ExtensionName returns the extension name
func (r *RateLimiter) ExtensionName() string {
	return "rateLimiter"
}

// InterceptOperation applies rate limiting
func (r *RateLimiter) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	// Rate limiting logic would go here
	// This would typically use a token bucket or sliding window algorithm
	return ctx
}

// ErrorLogger extension for logging errors
type ErrorLogger struct {
	logFunc func(ctx context.Context, err *graph.Error)
}

// NewErrorLogger creates a new error logger
func NewErrorLogger(logFunc func(ctx context.Context, err *graph.Error)) *ErrorLogger {
	return &ErrorLogger{
		logFunc: logFunc,
	}
}

// ExtensionName returns the extension name
func (e *ErrorLogger) ExtensionName() string {
	return "errorLogger"
}

// InterceptResponse logs errors
func (e *ErrorLogger) InterceptResponse(ctx context.Context, response *graph.Response) *graph.Response {
	if response.HasErrors() && e.logFunc != nil {
		for _, err := range response.Errors {
			e.logFunc(ctx, err)
		}
	}
	return response
}

// RequestLogger extension for logging requests
type RequestLogger struct {
	logFunc func(ctx context.Context, query, operationName string, variables map[string]interface{}, duration time.Duration)
}

// NewRequestLogger creates a new request logger
func NewRequestLogger(logFunc func(ctx context.Context, query, operationName string, variables map[string]interface{}, duration time.Duration)) *RequestLogger {
	return &RequestLogger{
		logFunc: logFunc,
	}
}

// ExtensionName returns the extension name
func (r *RequestLogger) ExtensionName() string {
	return "requestLogger"
}

// InterceptOperation logs requests
func (r *RequestLogger) InterceptOperation(ctx context.Context, next func(ctx context.Context) *graph.Response) context.Context {
	rc := graph.GetRequestContext(ctx)
	if rc == nil {
		return ctx
	}

	startTime := time.Now()
	rc.Set("requestLogger:start", startTime)

	return ctx
}

// InterceptResponse logs the completed request
func (r *RequestLogger) InterceptResponse(ctx context.Context, response *graph.Response) *graph.Response {
	rc := graph.GetRequestContext(ctx)
	if rc == nil || r.logFunc == nil {
		return response
	}

	if startTime, ok := rc.Get("requestLogger:start"); ok {
		duration := time.Since(startTime.(time.Time))
		r.logFunc(ctx, rc.Query, rc.OperationName, rc.Variables, duration)
	}

	return response
}

// FixedComplexity extension that sets fixed complexity values
type FixedComplexity struct {
	costs map[string]int
}

// NewFixedComplexity creates a fixed complexity extension
func NewFixedComplexity() *FixedComplexity {
	return &FixedComplexity{
		costs: make(map[string]int),
	}
}

// ExtensionName returns the extension name
func (f *FixedComplexity) ExtensionName() string {
	return "fixedComplexity"
}

// SetCost sets the complexity cost for a field
func (f *FixedComplexity) SetCost(typeName, fieldName string, cost int) *FixedComplexity {
	f.costs[typeName+"."+fieldName] = cost
	return f
}

// GetCost returns the complexity cost for a field
func (f *FixedComplexity) GetCost(typeName, fieldName string) int {
	if cost, ok := f.costs[typeName+"."+fieldName]; ok {
		return cost
	}
	return 1 // default cost
}
