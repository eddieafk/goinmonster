package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/eddieafk/goinmonster/graph"
)

// Server is the main GraphQL HTTP server
type Server struct {
	mu sync.RWMutex

	executableSchema *graph.ExecutableSchema
	transports       []Transport
	extensions       []Extension
	errorPresenter   ErrorPresenterFunc
	recoverFunc      RecoverFunc

	// Configuration
	queryCache           QueryCache
	complexityLimit      int
	requestTimeout       time.Duration
	enableIntrospection  bool
	enablePlayground     bool
	playgroundPath       string
	disableSuggestions   bool
	websocketUpgrader    WebsocketUpgrader
	websocketInitTimeout time.Duration
	websocketKeepAlive   time.Duration
}

// Config holds server configuration
type Config struct {
	EnableIntrospection  bool
	EnablePlayground     bool
	PlaygroundPath       string
	RequestTimeout       time.Duration
	ComplexityLimit      int
	DisableSuggestions   bool
	WebsocketInitTimeout time.Duration
	WebsocketKeepAlive   time.Duration
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		EnableIntrospection:  true,
		EnablePlayground:     true,
		PlaygroundPath:       "/playground",
		RequestTimeout:       30 * time.Second,
		ComplexityLimit:      0, // no limit
		DisableSuggestions:   false,
		WebsocketInitTimeout: 15 * time.Second,
		WebsocketKeepAlive:   30 * time.Second,
	}
}

// New creates a new GraphQL server
func New(es *graph.ExecutableSchema) *Server {
	return NewWithConfig(es, DefaultConfig())
}

// NewWithConfig creates a new server with custom configuration
func NewWithConfig(es *graph.ExecutableSchema, cfg Config) *Server {
	s := &Server{
		executableSchema:     es,
		transports:           make([]Transport, 0),
		extensions:           make([]Extension, 0),
		enableIntrospection:  cfg.EnableIntrospection,
		enablePlayground:     cfg.EnablePlayground,
		playgroundPath:       cfg.PlaygroundPath,
		requestTimeout:       cfg.RequestTimeout,
		complexityLimit:      cfg.ComplexityLimit,
		disableSuggestions:   cfg.DisableSuggestions,
		websocketInitTimeout: cfg.WebsocketInitTimeout,
		websocketKeepAlive:   cfg.WebsocketKeepAlive,
	}

	// Set default error presenter
	s.errorPresenter = DefaultErrorPresenter

	// Set default recover function
	s.recoverFunc = DefaultRecoverFunc

	return s
}

// Use adds an extension to the server
func (s *Server) Use(extension Extension) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extensions = append(s.extensions, extension)
}

// AddTransport adds a transport to the server
func (s *Server) AddTransport(transport Transport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transports = append(s.transports, transport)
}

// SetErrorPresenter sets a custom error presenter
func (s *Server) SetErrorPresenter(f ErrorPresenterFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errorPresenter = f
}

// SetRecoverFunc sets a custom recovery function
func (s *Server) SetRecoverFunc(f RecoverFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoverFunc = f
}

// SetQueryCache sets a query cache
func (s *Server) SetQueryCache(cache QueryCache) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryCache = cache
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Handle playground
	if s.enablePlayground && r.URL.Path == s.playgroundPath && r.Method == http.MethodGet {
		s.servePlayground(w, r)
		return
	}

	// Create request context
	ctx := r.Context()
	if s.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.requestTimeout)
		defer cancel()
	}

	// Find a transport that supports this request
	s.mu.RLock()
	transports := s.transports
	s.mu.RUnlock()

	for _, transport := range transports {
		if transport.Supports(r) {
			s.handleRequest(ctx, w, r, transport)
			return
		}
	}

	// No transport found
	http.Error(w, "unsupported transport", http.StatusBadRequest)
}

// handleRequest handles a GraphQL request with the given transport
func (s *Server) handleRequest(ctx context.Context, w http.ResponseWriter, r *http.Request, transport Transport) {
	defer func() {
		if rec := recover(); rec != nil {
			s.mu.RLock()
			recoverFunc := s.recoverFunc
			s.mu.RUnlock()

			err := recoverFunc(ctx, rec)
			s.writeError(w, err)
		}
	}()

	// Parse request
	params, err := transport.ParseRequest(r)
	if err != nil {
		s.writeError(w, err)
		return
	}

	// Execute operation hooks
	s.mu.RLock()
	extensions := s.extensions
	s.mu.RUnlock()

	// Create operation context
	rc := graph.NewRequestContext()
	rc.Query = params.Query
	rc.OperationName = params.OperationName
	rc.Variables = params.Variables
	ctx = graph.WithRequestContext(ctx, rc)

	// Call extension hooks: OperationStart
	for _, ext := range extensions {
		if hook, ok := ext.(OperationInterceptor); ok {
			ctx = hook.InterceptOperation(ctx, func(ctx context.Context) *graph.Response {
				return s.executeOperation(ctx, params)
			})
		}
	}

	// Execute operation
	response := s.executeOperation(ctx, params)

	// Call extension hooks: OperationEnd
	for _, ext := range extensions {
		if hook, ok := ext.(ResponseInterceptor); ok {
			response = hook.InterceptResponse(ctx, response)
		}
	}

	// Add extension data to response
	if response.Extensions == nil {
		response.Extensions = make(map[string]interface{})
	}
	for _, ext := range extensions {
		if hook, ok := ext.(ExtensionData); ok {
			for k, v := range hook.ExtensionData(ctx) {
				response.Extensions[k] = v
			}
		}
	}

	// Write response
	transport.WriteResponse(w, response)
}

// executeOperation executes a GraphQL operation
func (s *Server) executeOperation(ctx context.Context, params *RequestParams) *graph.Response {
	execParams := graph.ExecuteParams{
		Query:         params.Query,
		OperationName: params.OperationName,
		Variables:     params.Variables,
		Context:       ctx,
	}

	return s.executableSchema.Execute(ctx, execParams)
}

// writeError writes an error response
func (s *Server) writeError(w http.ResponseWriter, err error) {
	s.mu.RLock()
	presenter := s.errorPresenter
	s.mu.RUnlock()

	gqlErr := presenter(context.Background(), err)
	response := &graph.Response{
		Errors: []*graph.Error{gqlErr},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // GraphQL always returns 200
	// Debug: log the raw response for diagnosis
	debugBytes, _ := json.MarshalIndent(response, "", "  ")
	fmt.Println("--- GraphQL Response ---\n" + string(debugBytes) + "\n------------------------")
	json.NewEncoder(w).Encode(response)
}

// servePlayground serves the GraphQL Playground
func (s *Server) servePlayground(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(playgroundHTML))
}

// RequestParams contains parsed request parameters
type RequestParams struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
	Extensions    map[string]interface{} `json:"extensions"`
}

// ErrorPresenterFunc formats errors for response
type ErrorPresenterFunc func(ctx context.Context, err error) *graph.Error

// DefaultErrorPresenter is the default error presenter
func DefaultErrorPresenter(ctx context.Context, err error) *graph.Error {
	if gqlErr, ok := err.(*graph.Error); ok {
		return gqlErr
	}
	return &graph.Error{
		Message: err.Error(),
	}
}

// RecoverFunc handles panics
type RecoverFunc func(ctx context.Context, err interface{}) error

// DefaultRecoverFunc is the default recover function
func DefaultRecoverFunc(ctx context.Context, err interface{}) error {
	if e, ok := err.(error); ok {
		return e
	}
	return &graph.Error{Message: "internal server error"}
}

// QueryCache caches parsed queries
type QueryCache interface {
	Get(ctx context.Context, query string) (interface{}, bool)
	Set(ctx context.Context, query string, doc interface{})
}

// WebsocketUpgrader upgrades HTTP connections to WebSocket
type WebsocketUpgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (WebsocketConn, error)
}

// WebsocketConn represents a WebSocket connection
type WebsocketConn interface {
	ReadJSON(v interface{}) error
	WriteJSON(v interface{}) error
	Close() error
	SetReadLimit(limit int64)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// GetSchema returns the executable schema
func (s *Server) GetSchema() *graph.ExecutableSchema {
	return s.executableSchema
}

// WithResolvers sets the resolver map on the schema
func (s *Server) WithResolvers(rm *graph.ResolverMap) *Server {
	s.executableSchema.SetResolvers(rm)
	return s
}

// RegisterResolver registers a resolver for a type and field
func (s *Server) RegisterResolver(typeName, fieldName string, resolver graph.ResolverFunc) *Server {
	s.executableSchema.RegisterResolver(typeName, fieldName, resolver)
	return s
}

// GraphQL Playground HTML
const playgroundHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Apollo Sandbox</title>
  <style>
    body {
      height: 100vh;
      margin: 0;
      width: 100vw;
      overflow: hidden;
    }
  </style>
</head>
<body>
  <div style="width: 100%; height: 100%;" id="sandbox"></div>
  <script src="https://embeddable-sandbox.cdn.apollographql.com/_latest/embeddable-sandbox.umd.production.min.js"></script>
  <script>
    new window.EmbeddedSandbox({
      target: '#sandbox',
      initialEndpoint: '/graphql',
    });
  </script>
</body>
</html>`
