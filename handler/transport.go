package handler

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/eddieafk/goinmonster/graph"
)

// Transport defines how GraphQL requests are received and responses are sent
type Transport interface {
	// Supports returns true if this transport can handle the request
	Supports(r *http.Request) bool

	// ParseRequest parses the HTTP request into GraphQL parameters
	ParseRequest(r *http.Request) (*RequestParams, error)

	// WriteResponse writes the GraphQL response
	WriteResponse(w http.ResponseWriter, response *graph.Response)
}

// POST transport handles POST requests with JSON body
type POST struct {
	// MaxBodySize limits the request body size (default: 1MB)
	MaxBodySize int64
}

// NewPOST creates a new POST transport
func NewPOST() *POST {
	return &POST{
		MaxBodySize: 1024 * 1024, // 1MB default
	}
}

// Supports returns true for POST requests with JSON content type
func (t *POST) Supports(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return true // Assume JSON
	}

	mediaType, _, _ := mime.ParseMediaType(contentType)
	return mediaType == "application/json" || mediaType == "application/graphql"
}

// ParseRequest parses a POST request
func (t *POST) ParseRequest(r *http.Request) (*RequestParams, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)

	// Limit body size
	body := r.Body
	if t.MaxBodySize > 0 {
		body = http.MaxBytesReader(nil, body, t.MaxBodySize)
	}

	if mediaType == "application/graphql" {
		// Body is the query itself
		queryBytes, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		return &RequestParams{
			Query: string(queryBytes),
		}, nil
	}

	// Parse JSON body
	var params RequestParams
	if err := json.NewDecoder(body).Decode(&params); err != nil {
		return nil, err
	}

	return &params, nil
}

// WriteResponse writes a JSON response
func (t *POST) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// GET transport handles GET requests with query parameters
type GET struct {
	// MaxQueryLength limits the query string length
	MaxQueryLength int
}

// NewGET creates a new GET transport
func NewGET() *GET {
	return &GET{
		MaxQueryLength: 2048,
	}
}

// Supports returns true for GET requests
func (t *GET) Supports(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Query().Get("query") != ""
}

// ParseRequest parses a GET request
func (t *GET) ParseRequest(r *http.Request) (*RequestParams, error) {
	query := r.URL.Query()

	params := &RequestParams{
		Query:         query.Get("query"),
		OperationName: query.Get("operationName"),
	}

	// Parse variables if present
	if varsStr := query.Get("variables"); varsStr != "" {
		if err := json.Unmarshal([]byte(varsStr), &params.Variables); err != nil {
			return nil, err
		}
	}

	// Parse extensions if present
	if extStr := query.Get("extensions"); extStr != "" {
		if err := json.Unmarshal([]byte(extStr), &params.Extensions); err != nil {
			return nil, err
		}
	}

	return params, nil
}

// WriteResponse writes a JSON response
func (t *GET) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// MultipartForm transport handles multipart form uploads (for file uploads)
type MultipartForm struct {
	// MaxMemory is the maximum memory to use for parsing multipart
	MaxMemory int64
	// MaxUploadSize limits the total upload size
	MaxUploadSize int64
}

// NewMultipartForm creates a new multipart form transport
func NewMultipartForm() *MultipartForm {
	return &MultipartForm{
		MaxMemory:     32 << 20, // 32MB
		MaxUploadSize: 50 << 20, // 50MB
	}
}

// Supports returns true for multipart form requests
func (t *MultipartForm) Supports(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	return mediaType == "multipart/form-data"
}

// ParseRequest parses a multipart form request (GraphQL multipart spec)
func (t *MultipartForm) ParseRequest(r *http.Request) (*RequestParams, error) {
	if err := r.ParseMultipartForm(t.MaxMemory); err != nil {
		return nil, err
	}

	// Get operations field
	operations := r.FormValue("operations")
	if operations == "" {
		return nil, &graph.Error{Message: "missing operations field"}
	}

	var params RequestParams
	if err := json.Unmarshal([]byte(operations), &params); err != nil {
		return nil, err
	}

	// Get map field (for file mapping)
	mapField := r.FormValue("map")
	if mapField != "" {
		var fileMap map[string][]string
		if err := json.Unmarshal([]byte(mapField), &fileMap); err != nil {
			return nil, err
		}

		// Map files to variables
		for key, paths := range fileMap {
			file, header, err := r.FormFile(key)
			if err != nil {
				continue
			}

			upload := &Upload{
				File:     file,
				Filename: header.Filename,
				Size:     header.Size,
				MimeType: header.Header.Get("Content-Type"),
			}

			for _, path := range paths {
				setNestedValue(params.Variables, path, upload)
			}
		}
	}

	return &params, nil
}

// WriteResponse writes a JSON response
func (t *MultipartForm) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Upload represents a file upload
type Upload struct {
	File     io.ReadCloser
	Filename string
	Size     int64
	MimeType string
}

// setNestedValue sets a value in a nested map/slice structure using a path
func setNestedValue(v interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	current := v

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - set the value
			if m, ok := current.(map[string]interface{}); ok {
				m[part] = value
			}
			return
		}

		// Navigate to next level
		if m, ok := current.(map[string]interface{}); ok {
			if next, ok := m[part]; ok {
				current = next
			} else {
				// Create new map
				newMap := make(map[string]interface{})
				m[part] = newMap
				current = newMap
			}
		} else if s, ok := current.([]interface{}); ok {
			// Handle array index
			var idx int
			for j := 0; j < len(part); j++ {
				if part[j] >= '0' && part[j] <= '9' {
					idx = idx*10 + int(part[j]-'0')
				}
			}
			if idx < len(s) {
				current = s[idx]
			}
		}
	}
}

// OPTIONS transport handles CORS preflight requests
type OPTIONS struct{}

// NewOPTIONS creates a new OPTIONS transport
func NewOPTIONS() *OPTIONS {
	return &OPTIONS{}
}

// Supports returns true for OPTIONS requests
func (t *OPTIONS) Supports(r *http.Request) bool {
	return r.Method == http.MethodOptions
}

// ParseRequest returns empty params for OPTIONS
func (t *OPTIONS) ParseRequest(r *http.Request) (*RequestParams, error) {
	return &RequestParams{}, nil
}

// WriteResponse writes CORS headers
func (t *OPTIONS) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	w.Header().Set("Allow", "OPTIONS, GET, POST")
	w.WriteHeader(http.StatusOK)
}

// SSE transport handles Server-Sent Events (for subscriptions)
type SSE struct{}

// NewSSE creates a new SSE transport
func NewSSE() *SSE {
	return &SSE{}
}

// Supports returns true for SSE requests
func (t *SSE) Supports(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "text/event-stream")
}

// ParseRequest parses an SSE request
func (t *SSE) ParseRequest(r *http.Request) (*RequestParams, error) {
	return NewGET().ParseRequest(r)
}

// WriteResponse is not used for SSE (streaming is handled differently)
func (t *SSE) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	// SSE uses streaming, this is a fallback for errors
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	data, _ := json.Marshal(response)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// Batch transport handles batched GraphQL requests
type Batch struct {
	// MaxBatchSize limits the number of operations in a batch
	MaxBatchSize int
	// Underlying transport for parsing individual requests
	underlying Transport
}

// NewBatch creates a new batch transport
func NewBatch() *Batch {
	return &Batch{
		MaxBatchSize: 20,
		underlying:   NewPOST(),
	}
}

// Supports returns true for batch requests (JSON array)
func (t *Batch) Supports(r *http.Request) bool {
	if !t.underlying.Supports(r) {
		return false
	}
	// Check if content looks like a batch (array)
	// This is a simplified check - real implementation would peek at the body
	return r.Header.Get("X-Batch-Request") == "true"
}

// ParseRequest parses a batch request (returns first operation for now)
func (t *Batch) ParseRequest(r *http.Request) (*RequestParams, error) {
	// For batch requests, we'd parse an array and return them all
	// This is a simplified version
	return t.underlying.ParseRequest(r)
}

// WriteResponse writes a batch response
func (t *Batch) WriteResponse(w http.ResponseWriter, response *graph.Response) {
	t.underlying.WriteResponse(w, response)
}
