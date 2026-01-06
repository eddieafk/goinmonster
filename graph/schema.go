package graph

import (
	"fmt"
	"sync"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// Schema represents a parsed GraphQL schema with type information
type Schema struct {
	schema *ast.Schema
	source *ast.Source

	// Type mappings
	typeMap      map[string]*ObjectType
	inputTypeMap map[string]*InputType
	enumMap      map[string]*EnumType
	scalarMap    map[string]*ScalarType

	mu sync.RWMutex
}

// ObjectType represents a GraphQL object type with field definitions
type ObjectType struct {
	Name        string
	Description string
	Fields      map[string]*FieldDefinition
	Implements  []string
	Directives  []*Directive
}

// FieldDefinition represents a field in an object type
type FieldDefinition struct {
	Name        string
	Description string
	Type        *TypeRef
	Arguments   map[string]*ArgumentDefinition
	Directives  []*Directive

	// SQL mapping (optional)
	SQLColumn   string // Maps to SQL column name
	SQLTable    string // Maps to SQL table name
	SQLRelation string // Relation type: "hasOne", "hasMany", "belongsTo"
}

// ArgumentDefinition represents an argument for a field
type ArgumentDefinition struct {
	Name         string
	Description  string
	Type         *TypeRef
	DefaultValue interface{}
}

// TypeRef represents a reference to a type (can be list, non-null, etc.)
type TypeRef struct {
	Name     string
	NonNull  bool
	IsList   bool
	ListElem *TypeRef // For list types
}

// InputType represents a GraphQL input object type
type InputType struct {
	Name        string
	Description string
	Fields      map[string]*InputFieldDefinition
}

// InputFieldDefinition represents a field in an input type
type InputFieldDefinition struct {
	Name         string
	Description  string
	Type         *TypeRef
	DefaultValue interface{}
}

// EnumType represents a GraphQL enum type
type EnumType struct {
	Name        string
	Description string
	Values      []EnumValue
}

// EnumValue represents a single enum value
type EnumValue struct {
	Name        string
	Description string
	Value       interface{}
}

// ScalarType represents a custom scalar type
type ScalarType struct {
	Name        string
	Description string
	Marshaler   Marshaler
}

// Directive represents a directive attached to a schema element
type Directive struct {
	Name      string
	Arguments map[string]interface{}
}

// Marshaler interface for custom scalar types
type Marshaler interface {
	MarshalGraphQL(v interface{}) (interface{}, error)
	UnmarshalGraphQL(v interface{}) (interface{}, error)
}

// NewSchema creates a new schema from GraphQL SDL
func NewSchema(schemaString string) (*Schema, error) {
	source := &ast.Source{
		Name:  "schema.graphql",
		Input: schemaString,
	}

	schema, err := gqlparser.LoadSchema(source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	s := &Schema{
		schema:       schema,
		source:       source,
		typeMap:      make(map[string]*ObjectType),
		inputTypeMap: make(map[string]*InputType),
		enumMap:      make(map[string]*EnumType),
		scalarMap:    make(map[string]*ScalarType),
	}

	if err := s.buildTypeMap(); err != nil {
		return nil, err
	}

	return s, nil
}

// NewSchemaFromSources creates a schema from multiple SDL sources
func NewSchemaFromSources(sources ...*ast.Source) (*Schema, error) {
	schema, err := gqlparser.LoadSchema(sources...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse schema: %w", err)
	}

	s := &Schema{
		schema:       schema,
		typeMap:      make(map[string]*ObjectType),
		inputTypeMap: make(map[string]*InputType),
		enumMap:      make(map[string]*EnumType),
		scalarMap:    make(map[string]*ScalarType),
	}

	if err := s.buildTypeMap(); err != nil {
		return nil, err
	}

	return s, nil
}

// buildTypeMap builds internal type maps from the parsed schema
func (s *Schema) buildTypeMap() error {
	for name, def := range s.schema.Types {
		switch def.Kind {
		case ast.Object:
			objType := &ObjectType{
				Name:        name,
				Description: def.Description,
				Fields:      make(map[string]*FieldDefinition),
				Implements:  def.Interfaces,
				Directives:  convertDirectives(def.Directives),
			}

			for _, field := range def.Fields {
				objType.Fields[field.Name] = &FieldDefinition{
					Name:        field.Name,
					Description: field.Description,
					Type:        convertTypeRef(field.Type),
					Arguments:   convertArguments(field.Arguments),
					Directives:  convertDirectives(field.Directives),
				}

				// Extract SQL mapping from directives
				for _, dir := range field.Directives {
					if dir.Name == "sql" {
						for _, arg := range dir.Arguments {
							switch arg.Name {
							case "column":
								objType.Fields[field.Name].SQLColumn = arg.Value.Raw
							case "table":
								objType.Fields[field.Name].SQLTable = arg.Value.Raw
							case "relation":
								objType.Fields[field.Name].SQLRelation = arg.Value.Raw
							}
						}
					}
				}
			}

			s.typeMap[name] = objType

		case ast.InputObject:
			inputType := &InputType{
				Name:        name,
				Description: def.Description,
				Fields:      make(map[string]*InputFieldDefinition),
			}

			for _, field := range def.Fields {
				inputType.Fields[field.Name] = &InputFieldDefinition{
					Name:        field.Name,
					Description: field.Description,
					Type:        convertTypeRef(field.Type),
					DefaultValue: func() interface{} {
						if field.DefaultValue != nil {
							return field.DefaultValue.Raw
						}
						return nil
					}(),
				}
			}

			s.inputTypeMap[name] = inputType

		case ast.Enum:
			enumType := &EnumType{
				Name:        name,
				Description: def.Description,
				Values:      make([]EnumValue, 0, len(def.EnumValues)),
			}

			for _, val := range def.EnumValues {
				enumType.Values = append(enumType.Values, EnumValue{
					Name:        val.Name,
					Description: val.Description,
					Value:       val.Name,
				})
			}

			s.enumMap[name] = enumType

		case ast.Scalar:
			s.scalarMap[name] = &ScalarType{
				Name:        name,
				Description: def.Description,
			}
		}
	}

	return nil
}

// GetSchema returns the underlying gqlparser schema
func (s *Schema) GetSchema() *ast.Schema {
	return s.schema
}

// GetType returns an object type by name
func (s *Schema) GetType(name string) (*ObjectType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.typeMap[name]
	return t, ok
}

// GetInputType returns an input type by name
func (s *Schema) GetInputType(name string) (*InputType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.inputTypeMap[name]
	return t, ok
}

// GetEnum returns an enum type by name
func (s *Schema) GetEnum(name string) (*EnumType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.enumMap[name]
	return t, ok
}

// GetScalar returns a scalar type by name
func (s *Schema) GetScalar(name string) (*ScalarType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.scalarMap[name]
	return t, ok
}

// RegisterScalar registers a custom scalar type with a marshaler
func (s *Schema) RegisterScalar(name string, marshaler Marshaler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if scalar, ok := s.scalarMap[name]; ok {
		scalar.Marshaler = marshaler
	} else {
		s.scalarMap[name] = &ScalarType{
			Name:      name,
			Marshaler: marshaler,
		}
	}
}

// QueryType returns the Query type if defined
func (s *Schema) QueryType() (*ObjectType, bool) {
	if s.schema.Query == nil {
		return nil, false
	}
	return s.GetType(s.schema.Query.Name)
}

// MutationType returns the Mutation type if defined
func (s *Schema) MutationType() (*ObjectType, bool) {
	if s.schema.Mutation == nil {
		return nil, false
	}
	return s.GetType(s.schema.Mutation.Name)
}

// SubscriptionType returns the Subscription type if defined
func (s *Schema) SubscriptionType() (*ObjectType, bool) {
	if s.schema.Subscription == nil {
		return nil, false
	}
	return s.GetType(s.schema.Subscription.Name)
}

// convertTypeRef converts gqlparser type to our TypeRef
func convertTypeRef(t *ast.Type) *TypeRef {
	if t == nil {
		return nil
	}

	ref := &TypeRef{
		NonNull: t.NonNull,
	}

	if t.Elem != nil {
		ref.IsList = true
		ref.ListElem = convertTypeRef(t.Elem)
	} else {
		ref.Name = t.NamedType
	}

	return ref
}

// convertArguments converts gqlparser arguments to our ArgumentDefinition map
func convertArguments(args ast.ArgumentDefinitionList) map[string]*ArgumentDefinition {
	result := make(map[string]*ArgumentDefinition)

	for _, arg := range args {
		result[arg.Name] = &ArgumentDefinition{
			Name:        arg.Name,
			Description: arg.Description,
			Type:        convertTypeRef(arg.Type),
			DefaultValue: func() interface{} {
				if arg.DefaultValue != nil {
					return arg.DefaultValue.Raw
				}
				return nil
			}(),
		}
	}

	return result
}

// convertDirectives converts gqlparser directives to our Directive slice
func convertDirectives(dirs ast.DirectiveList) []*Directive {
	result := make([]*Directive, 0, len(dirs))

	for _, dir := range dirs {
		d := &Directive{
			Name:      dir.Name,
			Arguments: make(map[string]interface{}),
		}

		for _, arg := range dir.Arguments {
			d.Arguments[arg.Name] = arg.Value.Raw
		}

		result = append(result, d)
	}

	return result
}
