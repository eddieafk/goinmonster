package graph

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// Executor handles GraphQL query execution
type Executor struct {
	schema       *Schema
	resolverMap  *ResolverMap
	rootResolver RootResolver
	middleware   []MiddlewareFunc
	mu           sync.RWMutex

	// AST cache: map[query string] *ast.QueryDocument
	astCache sync.Map // map[string]*ast.QueryDocument
}

// NewExecutor creates a new query executor
func NewExecutor(schema *Schema) *Executor {
	return &Executor{
		schema:      schema,
		resolverMap: NewResolverMap(),
		middleware:  make([]MiddlewareFunc, 0),
	}
}

// SetResolverMap sets the resolver map
func (e *Executor) SetResolverMap(rm *ResolverMap) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.resolverMap = rm
}

// SetRootResolver sets the root resolver
func (e *Executor) SetRootResolver(resolver RootResolver) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rootResolver = resolver
}

// Use adds middleware to the executor
func (e *Executor) Use(mw MiddlewareFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.middleware = append(e.middleware, mw)
}

// ExecuteParams contains parameters for query execution
type ExecuteParams struct {
	Query         string
	OperationName string
	Variables     map[string]interface{}
	Context       context.Context
	RootValue     interface{}
}

// Execute executes a GraphQL operation
func (e *Executor) Execute(params ExecuteParams) *Response {
	ctx := params.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Create request context
	rc := NewRequestContext()
	rc.Query = params.Query
	rc.OperationName = params.OperationName
	rc.Variables = params.Variables
	ctx = WithRequestContext(ctx, rc)

	// Parse the query
	doc, err := e.parseQuery(params.Query)
	if err != nil {
		rc.AddError(&Error{Message: err.Error()})
		return NewResponse(rc)
	}

	// Find the operation to execute
	operation, err := e.findOperation(doc, params.OperationName)
	if err != nil {
		rc.AddError(&Error{Message: err.Error()})
		return NewResponse(rc)
	}

	// Build fragment map
	fragments := make(map[string]*ast.FragmentDefinition)
	for _, def := range doc.Fragments {
		fragments[def.Name] = def
	}

	// Create operation context
	opCtx := &OperationContext{
		OperationType: string(operation.Operation),
		OperationName: operation.Name,
		Variables:     params.Variables,
		Schema:        e.schema,
		RootResolver:  e.rootResolver,
	}
	rc.Operation = opCtx
	ctx = WithOperationContext(ctx, opCtx)

	// Collect fields
	collector := NewFieldCollector(e.schema, fragments, params.Variables)

	var rootType string
	switch operation.Operation {
	case ast.Query:
		rootType = "Query"
	case ast.Mutation:
		rootType = "Mutation"
	case ast.Subscription:
		rootType = "Subscription"
	}

	selections := collector.CollectFields(operation.SelectionSet, rootType)

	// Execute the operation
	data, err := e.executeSelectionSet(ctx, selections, rootType, params.RootValue, nil)
	if err != nil {
		rc.AddError(&Error{Message: err.Error()})
	}

	rc.Data = data
	return NewResponse(rc)
}

// parseQuery parses a GraphQL query document
func (e *Executor) parseQuery(query string) (*ast.QueryDocument, error) {
	// Try cache first
	if cached, ok := e.astCache.Load(query); ok {
		if doc, ok := cached.(*ast.QueryDocument); ok && doc != nil {
			return doc, nil
		}
	}

	doc, errs := gqlparser.LoadQuery(e.schema.GetSchema(), query)
	if len(errs) > 0 {
		return nil, errs[0]
	}

	e.astCache.Store(query, doc)
	return doc, nil
}

// findOperation finds the operation to execute
func (e *Executor) findOperation(doc *ast.QueryDocument, operationName string) (*ast.OperationDefinition, error) {
	if len(doc.Operations) == 0 {
		return nil, fmt.Errorf("no operations in document")
	}

	if operationName == "" {
		if len(doc.Operations) == 1 {
			return doc.Operations[0], nil
		}
		return nil, fmt.Errorf("operation name is required when document contains multiple operations")
	}

	for _, op := range doc.Operations {
		if op.Name == operationName {
			return op, nil
		}
	}

	return nil, fmt.Errorf("operation %q not found", operationName)
}

// executeSelectionSet executes a selection set against a parent object
func (e *Executor) executeSelectionSet(
	ctx context.Context,
	selections *SelectionSet,
	parentType string,
	parentValue interface{},
	path []interface{},
) (map[string]interface{}, error) {
	if selections == nil || len(selections.Fields) == 0 {
		return nil, nil
	}

	result := make(map[string]interface{})

	for _, field := range selections.Fields {
		fieldPath := append(path, field.GetName())

		value, err := e.executeField(ctx, field, parentType, parentValue, fieldPath)
		if err != nil {
			// Continue execution but record error
			rc := GetRequestContext(ctx)
			if rc != nil {
				rc.AddError(&Error{
					Message: err.Error(),
					Path:    fieldPath,
				})
			}
			result[field.GetName()] = nil
			continue
		}

		result[field.GetName()] = value
	}

	// Add __typename if requested
	if selections.Typename {
		result["__typename"] = parentType
	}

	return result, nil
}

// executeField executes a single field
func (e *Executor) executeField(
	ctx context.Context,
	field *SelectedField,
	parentType string,
	parentValue interface{},
	path []interface{},
) (interface{}, error) {
	// Handle introspection fields
	if field.Name == "__schema" {
		return e.introspectSchema(ctx, field)
	}
	if field.Name == "__type" {
		typeName, _ := field.Arguments["name"].(string)
		return e.introspectType(ctx, field, typeName)
	}
	if field.Name == "__typename" {
		return parentType, nil
	}

	// Build resolve info
	info := &ResolveInfo{
		FieldName:  field.Name,
		ParentType: parentType,
		Arguments:  field.Arguments,
		Variables:  GetRequestContext(ctx).Variables,
		Selection:  field.Selections,
		Path:       toStringPath(path),
		RootValue:  GetRequestContext(ctx).Data,
	}

	if opCtx := GetOperationContext(ctx); opCtx != nil {
		info.OperationCtx = opCtx
	}

	// Get the field type
	objType, _ := e.schema.GetType(parentType)
	if objType != nil {
		if fieldDef, ok := objType.Fields[field.Name]; ok {
			info.ReturnType = fieldDef.Type
		}
	}

	ctx = WithResolveInfo(ctx, info)

	// Try to resolve the field
	var value interface{}
	var err error

	// Check for registered resolver
	e.mu.RLock()
	resolver, hasResolver := e.resolverMap.Get(parentType, field.Name)
	e.mu.RUnlock()

	if hasResolver {
		value, err = resolver.Resolve(ctx, field.Arguments)
	} else {
		// Default field resolution (from parent value)
		value, err = e.defaultResolve(parentValue, field.Name)
	}

	if err != nil {
		return nil, err
	}

	// Complete the value (handle lists, objects, etc.)
	return e.completeValue(ctx, field, parentType, value, path)
}

// defaultResolve resolves a field from the parent value using reflection
func (e *Executor) defaultResolve(parent interface{}, fieldName string) (interface{}, error) {
	if parent == nil {
		return nil, nil
	}

	val := reflect.ValueOf(parent)

	// Handle pointers
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, nil
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		// Try to get value from map
		if val.Type().Key().Kind() == reflect.String {
			mapVal := val.MapIndex(reflect.ValueOf(fieldName))
			if mapVal.IsValid() {
				return mapVal.Interface(), nil
			}
		}

	case reflect.Struct:
		// Try field by name
		fieldVal := val.FieldByName(fieldName)
		if fieldVal.IsValid() && fieldVal.CanInterface() {
			return fieldVal.Interface(), nil
		}

		// Try field by tag
		for i := 0; i < val.NumField(); i++ {
			field := val.Type().Field(i)
			if tag := field.Tag.Get("json"); tag == fieldName {
				return val.Field(i).Interface(), nil
			}
			if tag := field.Tag.Get("graphql"); tag == fieldName {
				return val.Field(i).Interface(), nil
			}
		}

		// Try method
		method := reflect.ValueOf(parent).MethodByName(capitalize(fieldName))
		if method.IsValid() {
			results := method.Call(nil)
			if len(results) > 0 {
				return results[0].Interface(), nil
			}
		}
	}

	return nil, nil
}

// completeValue completes the value resolution
func (e *Executor) completeValue(
	ctx context.Context,
	field *SelectedField,
	parentType string,
	value interface{},
	path []interface{},
) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	val := reflect.ValueOf(value)

	// Handle pointers
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, nil
		}
		val = val.Elem()
		value = val.Interface()
	}

	// Handle slices/arrays
	if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
		return e.completeListValue(ctx, field, parentType, val, path)
	}

	// Handle maps and structs (object types)
	if val.Kind() == reflect.Map || val.Kind() == reflect.Struct {
		if field.HasSelection() {
			fieldType := e.getFieldTypeName(parentType, field.Name)
			return e.executeSelectionSet(ctx, field.Selections, fieldType, value, path)
		}
	}

	// Scalar values - return as-is
	return value, nil
}

// completeListValue completes a list value
func (e *Executor) completeListValue(
	ctx context.Context,
	field *SelectedField,
	parentType string,
	val reflect.Value,
	path []interface{},
) ([]interface{}, error) {
	result := make([]interface{}, val.Len())

	for i := 0; i < val.Len(); i++ {
		itemPath := append(path, i)
		item := val.Index(i).Interface()

		completed, err := e.completeValue(ctx, field, parentType, item, itemPath)
		if err != nil {
			return nil, err
		}

		result[i] = completed
	}

	return result, nil
}

// getFieldTypeName gets the return type name for a field
func (e *Executor) getFieldTypeName(parentType, fieldName string) string {
	objType, ok := e.schema.GetType(parentType)
	if !ok {
		return ""
	}

	field, ok := objType.Fields[fieldName]
	if !ok {
		return ""
	}

	return unwrapTypeName(field.Type)
}

// unwrapTypeName unwraps a TypeRef to get the underlying type name
func unwrapTypeName(t *TypeRef) string {
	if t == nil {
		return ""
	}

	if t.IsList {
		return unwrapTypeName(t.ListElem)
	}

	return t.Name
}

// toStringPath converts an interface path to a string path
func toStringPath(path []interface{}) []string {
	result := make([]string, len(path))
	for i, p := range path {
		result[i] = fmt.Sprintf("%v", p)
	}
	return result
}

// capitalize capitalizes the first letter
func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return string(s[0]-32) + s[1:]
}

// ExecutableSchema combines schema and executor for execution
type ExecutableSchema struct {
	Schema   *Schema
	Executor *Executor
}

// NewExecutableSchema creates a new executable schema
func NewExecutableSchema(schemaString string) (*ExecutableSchema, error) {
	schema, err := NewSchema(schemaString)
	if err != nil {
		return nil, err
	}

	executor := NewExecutor(schema)

	return &ExecutableSchema{
		Schema:   schema,
		Executor: executor,
	}, nil
}

// Execute executes a GraphQL operation
func (es *ExecutableSchema) Execute(ctx context.Context, params ExecuteParams) *Response {
	params.Context = ctx
	return es.Executor.Execute(params)
}

// SetResolvers sets the resolver map
func (es *ExecutableSchema) SetResolvers(rm *ResolverMap) {
	es.Executor.SetResolverMap(rm)
}

// RegisterResolver registers a resolver for a type and field
func (es *ExecutableSchema) RegisterResolver(typeName, fieldName string, resolver ResolverFunc) {
	if es.Executor.resolverMap == nil {
		es.Executor.resolverMap = NewResolverMap()
	}
	es.Executor.resolverMap.Register(typeName, fieldName, resolver)
}

// Use adds middleware to the executor
func (es *ExecutableSchema) Use(mw MiddlewareFunc) {
	es.Executor.Use(mw)
}

// introspectSchema handles the __schema introspection query
func (e *Executor) introspectSchema(ctx context.Context, field *SelectedField) (map[string]interface{}, error) {
	schema := e.schema.GetSchema()
	result := make(map[string]interface{})

	// Process each requested field in the selection set
	if field.Selections != nil {
		for _, sel := range field.Selections.Fields {
			switch sel.Name {
			case "queryType":
				if schema.Query != nil {
					result["queryType"] = e.buildTypeRef(schema.Query, sel)
				}
			case "mutationType":
				if schema.Mutation != nil {
					result["mutationType"] = e.buildTypeRef(schema.Mutation, sel)
				} else {
					result["mutationType"] = nil
				}
			case "subscriptionType":
				if schema.Subscription != nil {
					result["subscriptionType"] = e.buildTypeRef(schema.Subscription, sel)
				} else {
					result["subscriptionType"] = nil
				}
			case "types":
				types := make([]map[string]interface{}, 0)
				for name, typeDef := range schema.Types {
					if len(name) >= 2 && name[:2] == "__" {
						continue // skip introspection types
					}
					types = append(types, e.buildFullType(typeDef, sel, name))
				}
				result["types"] = types
			case "directives":
				directives := make([]map[string]interface{}, 0)
				for _, dir := range schema.Directives {
					directives = append(directives, e.buildDirective(dir, sel))
				}
				result["directives"] = directives
			case "description":
				result["description"] = schema.Description
			}
		}
	}

	return result, nil
}

// introspectType handles the __type introspection query
func (e *Executor) introspectType(ctx context.Context, field *SelectedField, typeName string) (interface{}, error) {
	schema := e.schema.GetSchema()
	typeDef, ok := schema.Types[typeName]
	if !ok {
		return nil, nil
	}
	return e.buildFullType(typeDef, field, typeName), nil
}

// buildTypeRef builds a minimal type reference for introspection
func (e *Executor) buildTypeRef(def *ast.Definition, field *SelectedField) map[string]interface{} {
	result := make(map[string]interface{})
	// Always include name and kind
	result["name"] = def.Name
	result["kind"] = kindToIntrospection(def.Kind)
	if field.Selections != nil {
		for _, sel := range field.Selections.Fields {
			switch sel.Name {
			case "description":
				result["description"] = def.Description
			case "fields":
				result["fields"] = e.buildFieldList(def, sel)
			case "interfaces":
				result["interfaces"] = e.buildInterfaces(def, sel)
			case "possibleTypes":
				result["possibleTypes"] = nil
			case "enumValues":
				result["enumValues"] = e.buildEnumValues(def, sel)
			case "inputFields":
				result["inputFields"] = e.buildInputFields(def, sel)
			case "ofType":
				result["ofType"] = nil
			}
		}
	}
	return result
}

// buildFullType builds a complete type for introspection
func (e *Executor) buildFullType(def *ast.Definition, field *SelectedField, name string) map[string]interface{} {
	result := make(map[string]interface{})
	// Always include name and kind
	result["name"] = name
	result["kind"] = kindToIntrospection(def.Kind)
	result["description"] = def.Description

	if field.Selections != nil {
		for _, sel := range field.Selections.Fields {
			switch sel.Name {
			case "fields":
				result["fields"] = e.buildFieldList(def, sel)
			case "interfaces":
				result["interfaces"] = e.buildInterfaces(def, sel)
			case "possibleTypes":
				result["possibleTypes"] = e.buildPossibleTypes(def, sel)
			case "enumValues":
				result["enumValues"] = e.buildEnumValues(def, sel)
			case "inputFields":
				result["inputFields"] = e.buildInputFields(def, sel)
			case "ofType":
				result["ofType"] = nil
			}
		}
	}

	return result
}

// buildFieldList builds the fields list for introspection
func (e *Executor) buildFieldList(def *ast.Definition, field *SelectedField) interface{} {
	if def.Kind != ast.Object && def.Kind != ast.Interface {
		return nil
	}

	// Check includeDeprecated argument
	includeDeprecated := false
	if v, ok := field.Arguments["includeDeprecated"].(bool); ok {
		includeDeprecated = v
	}

	fields := make([]map[string]interface{}, 0)
	for _, f := range def.Fields {
		// Skip introspection fields from Query type
		if def.Name == "Query" && (f.Name == "__schema" || f.Name == "__type") {
			continue
		}
		if !includeDeprecated && isDeprecated(f.Directives) {
			continue
		}
		fields = append(fields, e.buildField(f, field))
	}
	return fields
}

// buildField builds a single field for introspection
func (e *Executor) buildField(f *ast.FieldDefinition, parentField *SelectedField) map[string]interface{} {
	result := map[string]interface{}{
		"name":              f.Name,
		"description":       f.Description,
		"isDeprecated":      isDeprecated(f.Directives),
		"deprecationReason": getDeprecationReason(f.Directives),
	}

	if parentField.Selections != nil {
		for _, sel := range parentField.Selections.Fields {
			switch sel.Name {
			case "args":
				args := make([]map[string]interface{}, 0)
				for _, arg := range f.Arguments {
					args = append(args, e.buildInputValue(arg, sel))
				}
				result["args"] = args
			case "type":
				result["type"] = e.buildTypeFromRef(f.Type, sel)
			}
		}
	}

	return result
}

// buildTypeFromRef builds a type from an ast.Type reference
func (e *Executor) buildTypeFromRef(t *ast.Type, field *SelectedField) map[string]interface{} {
	if t == nil {
		return nil
	}

	result := make(map[string]interface{})

	if t.NonNull {
		result["kind"] = "NON_NULL"
		result["name"] = nil
		if field.Selections != nil {
			for _, sel := range field.Selections.Fields {
				if sel.Name == "ofType" {
					// Create a copy of the type without NonNull
					innerType := *t
					innerType.NonNull = false
					result["ofType"] = e.buildTypeFromRef(&innerType, sel)
				}
			}
		}
		return result
	}

	if t.Elem != nil {
		result["kind"] = "LIST"
		result["name"] = nil
		if field.Selections != nil {
			for _, sel := range field.Selections.Fields {
				if sel.Name == "ofType" {
					result["ofType"] = e.buildTypeFromRef(t.Elem, sel)
				}
			}
		}
		return result
	}

	// Named type
	schema := e.schema.GetSchema()
	if typeDef, ok := schema.Types[t.NamedType]; ok {
		result["kind"] = kindToIntrospection(typeDef.Kind)
	} else {
		result["kind"] = "SCALAR"
	}
	result["name"] = t.NamedType
	result["ofType"] = nil

	return result
}

// buildInputValue builds an input value for introspection
func (e *Executor) buildInputValue(arg *ast.ArgumentDefinition, field *SelectedField) map[string]interface{} {
	result := map[string]interface{}{
		"name":        arg.Name,
		"description": arg.Description,
	}

	if field.Selections != nil {
		for _, sel := range field.Selections.Fields {
			switch sel.Name {
			case "type":
				result["type"] = e.buildTypeFromRef(arg.Type, sel)
			case "defaultValue":
				if arg.DefaultValue != nil {
					result["defaultValue"] = arg.DefaultValue.String()
				} else {
					result["defaultValue"] = nil
				}
			}
		}
	}

	return result
}

// buildInterfaces builds the interfaces list for introspection
func (e *Executor) buildInterfaces(def *ast.Definition, field *SelectedField) interface{} {
	if def.Kind != ast.Object {
		return nil
	}

	interfaces := make([]map[string]interface{}, 0)
	schema := e.schema.GetSchema()
	for _, iface := range def.Interfaces {
		if ifaceDef, ok := schema.Types[iface]; ok {
			interfaces = append(interfaces, e.buildTypeRef(ifaceDef, field))
		}
	}
	return interfaces
}

// buildPossibleTypes builds possible types for unions/interfaces
func (e *Executor) buildPossibleTypes(def *ast.Definition, field *SelectedField) interface{} {
	if def.Kind != ast.Interface && def.Kind != ast.Union {
		return nil
	}

	types := make([]map[string]interface{}, 0)
	schema := e.schema.GetSchema()

	if def.Kind == ast.Union {
		for _, t := range def.Types {
			if typeDef, ok := schema.Types[t]; ok {
				types = append(types, e.buildTypeRef(typeDef, field))
			}
		}
	} else {
		// For interfaces, find implementing types
		for _, typeDef := range schema.Types {
			if typeDef.Kind == ast.Object {
				for _, iface := range typeDef.Interfaces {
					if iface == def.Name {
						types = append(types, e.buildTypeRef(typeDef, field))
						break
					}
				}
			}
		}
	}

	return types
}

// buildEnumValues builds enum values for introspection
func (e *Executor) buildEnumValues(def *ast.Definition, field *SelectedField) interface{} {
	if def.Kind != ast.Enum {
		return nil
	}

	includeDeprecated := false
	if v, ok := field.Arguments["includeDeprecated"].(bool); ok {
		includeDeprecated = v
	}

	values := make([]map[string]interface{}, 0)
	for _, v := range def.EnumValues {
		if !includeDeprecated && isDeprecated(v.Directives) {
			continue
		}
		values = append(values, map[string]interface{}{
			"name":              v.Name,
			"description":       v.Description,
			"isDeprecated":      isDeprecated(v.Directives),
			"deprecationReason": getDeprecationReason(v.Directives),
		})
	}
	return values
}

// buildInputFields builds input fields for introspection
func (e *Executor) buildInputFields(def *ast.Definition, field *SelectedField) interface{} {
	if def.Kind != ast.InputObject {
		return nil
	}

	fields := make([]map[string]interface{}, 0)
	for _, f := range def.Fields {
		fields = append(fields, e.buildInputFieldDef(f, field))
	}
	return fields
}

// buildInputFieldDef builds an input field definition
func (e *Executor) buildInputFieldDef(f *ast.FieldDefinition, parentField *SelectedField) map[string]interface{} {
	result := map[string]interface{}{
		"name":        f.Name,
		"description": f.Description,
	}

	if parentField.Selections != nil {
		for _, sel := range parentField.Selections.Fields {
			switch sel.Name {
			case "type":
				result["type"] = e.buildTypeFromRef(f.Type, sel)
			case "defaultValue":
				if f.DefaultValue != nil {
					result["defaultValue"] = f.DefaultValue.String()
				} else {
					result["defaultValue"] = nil
				}
			}
		}
	}

	return result
}

// buildDirective builds a directive for introspection
func (e *Executor) buildDirective(dir *ast.DirectiveDefinition, field *SelectedField) map[string]interface{} {
	result := map[string]interface{}{
		"name":         dir.Name,
		"description":  dir.Description,
		"isRepeatable": dir.IsRepeatable,
	}

	if field.Selections != nil {
		for _, sel := range field.Selections.Fields {
			switch sel.Name {
			case "locations":
				locations := make([]string, 0)
				for _, loc := range dir.Locations {
					locations = append(locations, string(loc))
				}
				result["locations"] = locations
			case "args":
				args := make([]map[string]interface{}, 0)
				for _, arg := range dir.Arguments {
					args = append(args, e.buildInputValue(arg, sel))
				}
				result["args"] = args
			}
		}
	}

	return result
}

// isDeprecated checks if a field/enum is deprecated
func isDeprecated(directives ast.DirectiveList) bool {
	for _, d := range directives {
		if d.Name == "deprecated" {
			return true
		}
	}
	return false
}

// getDeprecationReason gets the deprecation reason from directives
func getDeprecationReason(directives ast.DirectiveList) interface{} {
	for _, d := range directives {
		if d.Name == "deprecated" {
			for _, arg := range d.Arguments {
				if arg.Name == "reason" {
					return arg.Value.Raw
				}
			}
			return "No longer supported"
		}
	}
	return nil
}

// kindToIntrospection converts gqlparser's DefinitionKind to GraphQL introspection kind
func kindToIntrospection(kind ast.DefinitionKind) string {
	switch kind {
	case ast.Scalar:
		return "SCALAR"
	case ast.Object:
		return "OBJECT"
	case ast.Interface:
		return "INTERFACE"
	case ast.Union:
		return "UNION"
	case ast.Enum:
		return "ENUM"
	case ast.InputObject:
		return "INPUT_OBJECT"
	default:
		return string(kind)
	}
}
