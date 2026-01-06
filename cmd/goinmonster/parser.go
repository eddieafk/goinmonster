package goinmonster

import (
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// SchemaAnalysis holds the analyzed schema information
type SchemaAnalysis struct {
	ObjectTypes    []ObjectTypeDef
	InputTypes     []InputTypeDef
	EnumTypes      []EnumTypeDef
	Scalars        []string
	QueryFields    []QueryFieldDef
	MutationFields []MutationFieldDef
}

// ObjectTypeDef represents a GraphQL object type
type ObjectTypeDef struct {
	Name   string
	Fields []FieldDef
}

// FieldDef represents a field in an object type
type FieldDef struct {
	Name      string
	TypeName  string
	IsList    bool
	IsNonNull bool
	Column    string   // from @sql(column: "...")
	Table     string   // from @sql(table: "...")
	Relation  string   // from @sql(relation: "...")
	Arguments []string // argument names
}

// InputTypeDef represents a GraphQL input type
type InputTypeDef struct {
	Name   string
	Fields []InputFieldDef
}

// InputFieldDef represents a field in an input type
type InputFieldDef struct {
	Name      string
	TypeName  string
	IsList    bool
	IsNonNull bool
}

// EnumTypeDef represents a GraphQL enum type
type EnumTypeDef struct {
	Name   string
	Values []string
}

// QueryFieldDef represents a query field
type QueryFieldDef struct {
	Name      string
	TypeName  string
	IsList    bool
	Arguments []string
}

// MutationFieldDef represents a mutation field
type MutationFieldDef struct {
	Name      string
	TypeName  string
	IsList    bool
	Arguments []string
}

// parseSchema parses the GraphQL schema string
func parseSchema(schemaStr string) (*ast.Schema, error) {
	source := &ast.Source{
		Name:  "schema.graphqls",
		Input: schemaStr,
	}

	schema, err := gqlparser.LoadSchema(source)
	if err != nil {
		return nil, err
	}

	return schema, nil
}

// analyzeSchema analyzes the parsed schema and extracts type information
func analyzeSchema(schema *ast.Schema, config *Config) *SchemaAnalysis {
	analysis := &SchemaAnalysis{}

	// Collect scalars
	for name := range schema.Types {
		typeDef := schema.Types[name]
		if typeDef.Kind == ast.Scalar && !isBuiltinScalar(name) {
			analysis.Scalars = append(analysis.Scalars, name)
		}
	}

	// Collect object types
	for name, typeDef := range schema.Types {
		if typeDef.Kind == ast.Object {
			if name == "__Schema" || name == "__Type" || name == "__Field" ||
				name == "__InputValue" || name == "__EnumValue" || name == "__Directive" {
				continue // skip introspection types
			}

			objType := ObjectTypeDef{
				Name:   name,
				Fields: make([]FieldDef, 0),
			}

			for _, field := range typeDef.Fields {
				fieldDef := analyzeField(field)
				objType.Fields = append(objType.Fields, fieldDef)
			}

			analysis.ObjectTypes = append(analysis.ObjectTypes, objType)
		}
	}

	// Collect input types
	for name, typeDef := range schema.Types {
		if typeDef.Kind == ast.InputObject {
			inputType := InputTypeDef{
				Name:   name,
				Fields: make([]InputFieldDef, 0),
			}

			for _, field := range typeDef.Fields {
				inputField := InputFieldDef{
					Name:      field.Name,
					TypeName:  getBaseTypeName(field.Type),
					IsList:    isListType(field.Type),
					IsNonNull: field.Type.NonNull,
				}
				inputType.Fields = append(inputType.Fields, inputField)
			}

			analysis.InputTypes = append(analysis.InputTypes, inputType)
		}
	}

	// Collect enum types
	for name, typeDef := range schema.Types {
		if typeDef.Kind == ast.Enum {
			if name == "__TypeKind" || name == "__DirectiveLocation" {
				continue // skip introspection enums
			}

			enumType := EnumTypeDef{
				Name:   name,
				Values: make([]string, 0),
			}

			for _, value := range typeDef.EnumValues {
				enumType.Values = append(enumType.Values, value.Name)
			}

			analysis.EnumTypes = append(analysis.EnumTypes, enumType)
		}
	}

	// Analyze Query type
	if queryType := schema.Query; queryType != nil {
		for _, field := range queryType.Fields {
			args := make([]string, 0)
			for _, arg := range field.Arguments {
				args = append(args, arg.Name)
			}

			analysis.QueryFields = append(analysis.QueryFields, QueryFieldDef{
				Name:      field.Name,
				TypeName:  getBaseTypeName(field.Type),
				IsList:    isListType(field.Type),
				Arguments: args,
			})
		}
	}

	// Analyze Mutation type
	if mutationType := schema.Mutation; mutationType != nil {
		for _, field := range mutationType.Fields {
			args := make([]string, 0)
			for _, arg := range field.Arguments {
				args = append(args, arg.Name)
			}

			analysis.MutationFields = append(analysis.MutationFields, MutationFieldDef{
				Name:      field.Name,
				TypeName:  getBaseTypeName(field.Type),
				IsList:    isListType(field.Type),
				Arguments: args,
			})
		}
	}

	return analysis
}

// analyzeField extracts field information including @sql directive data
func analyzeField(field *ast.FieldDefinition) FieldDef {
	fieldDef := FieldDef{
		Name:      field.Name,
		TypeName:  getBaseTypeName(field.Type),
		IsList:    isListType(field.Type),
		IsNonNull: field.Type.NonNull,
		Arguments: make([]string, 0),
	}

	// Extract argument names
	for _, arg := range field.Arguments {
		fieldDef.Arguments = append(fieldDef.Arguments, arg.Name)
	}

	// Look for @sql directive
	for _, directive := range field.Directives {
		if directive.Name == "sql" {
			for _, arg := range directive.Arguments {
				switch arg.Name {
				case "column":
					fieldDef.Column = arg.Value.Raw
				case "table":
					fieldDef.Table = arg.Value.Raw
				case "relation":
					fieldDef.Relation = arg.Value.Raw
				}
			}
		}
	}

	return fieldDef
}

// getBaseTypeName extracts the base type name from a type reference
func getBaseTypeName(t *ast.Type) string {
	if t == nil {
		return ""
	}

	// Handle NonNull wrapper
	if t.Elem != nil {
		return getBaseTypeName(t.Elem)
	}

	return t.NamedType
}

// isListType checks if the type is a list
func isListType(t *ast.Type) bool {
	if t == nil {
		return false
	}

	// If this is a NonNull type, check the inner type
	if t.NonNull && t.Elem != nil {
		return isListType(t.Elem)
	}

	// Check if it's a list
	if t.Elem != nil {
		return true
	}

	return false
}

// isBuiltinScalar checks if a scalar is a GraphQL built-in
func isBuiltinScalar(name string) bool {
	builtins := map[string]bool{
		"String":  true,
		"Int":     true,
		"Float":   true,
		"Boolean": true,
		"ID":      true,
	}
	return builtins[name]
}
