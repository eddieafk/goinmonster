package graph

import (
	"github.com/vektah/gqlparser/v2/ast"
)

// FieldCollector collects and processes fields from a GraphQL operation
type FieldCollector struct {
	schema    *Schema
	fragments map[string]*ast.FragmentDefinition
	variables map[string]interface{}
}

// NewFieldCollector creates a new field collector
func NewFieldCollector(schema *Schema, fragments map[string]*ast.FragmentDefinition, variables map[string]interface{}) *FieldCollector {
	return &FieldCollector{
		schema:    schema,
		fragments: fragments,
		variables: variables,
	}
}

// CollectFields collects fields from a selection set, handling fragments and spreads
func (fc *FieldCollector) CollectFields(selectionSet ast.SelectionSet, parentType string) *SelectionSet {
	if selectionSet == nil {
		return nil
	}

	result := &SelectionSet{
		Fields: make([]*SelectedField, 0),
	}

	// Track field names to merge duplicate selections
	fieldMap := make(map[string]*SelectedField)

	fc.collectFieldsImpl(selectionSet, parentType, fieldMap, result)

	// Convert map to slice maintaining order
	for _, field := range fieldMap {
		result.Fields = append(result.Fields, field)
	}

	return result
}

func (fc *FieldCollector) collectFieldsImpl(selectionSet ast.SelectionSet, parentType string, fieldMap map[string]*SelectedField, result *SelectionSet) {
	for _, selection := range selectionSet {
		switch sel := selection.(type) {
		case *ast.Field:
			if !fc.shouldInclude(sel.Directives) {
				continue
			}

			// Handle __typename specially
			if sel.Name == "__typename" {
				result.Typename = true
				continue
			}

			responseKey := sel.Alias
			if responseKey == "" {
				responseKey = sel.Name
			}

			// Check if field already exists (for merging)
			if existing, ok := fieldMap[responseKey]; ok {
				// Merge selections
				if sel.SelectionSet != nil {
					if existing.Selections == nil {
						existing.Selections = &SelectionSet{Fields: make([]*SelectedField, 0)}
					}
					nestedFields := fc.CollectFields(sel.SelectionSet, fc.getFieldTypeName(parentType, sel.Name))
					existing.Selections.Fields = append(existing.Selections.Fields, nestedFields.Fields...)
				}
			} else {
				field := &SelectedField{
					Name:       sel.Name,
					Alias:      sel.Alias,
					Arguments:  fc.collectArguments(sel.Arguments),
					Directives: fc.collectDirectives(sel.Directives),
				}

				// Recursively collect nested fields
				if sel.SelectionSet != nil {
					nestedType := fc.getFieldTypeName(parentType, sel.Name)
					field.Selections = fc.CollectFields(sel.SelectionSet, nestedType)
				}

				fieldMap[responseKey] = field
			}

		case *ast.FragmentSpread:
			if !fc.shouldInclude(sel.Directives) {
				continue
			}

			fragment, ok := fc.fragments[sel.Name]
			if !ok {
				continue
			}

			// Check if fragment applies to this type
			if !fc.typeApplies(fragment.TypeCondition, parentType) {
				continue
			}

			fc.collectFieldsImpl(fragment.SelectionSet, parentType, fieldMap, result)

		case *ast.InlineFragment:
			if !fc.shouldInclude(sel.Directives) {
				continue
			}

			// Check type condition if present
			if sel.TypeCondition != "" && !fc.typeApplies(sel.TypeCondition, parentType) {
				continue
			}

			fc.collectFieldsImpl(sel.SelectionSet, parentType, fieldMap, result)
		}
	}
}

// shouldInclude checks @skip and @include directives
func (fc *FieldCollector) shouldInclude(directives ast.DirectiveList) bool {
	for _, dir := range directives {
		switch dir.Name {
		case "skip":
			if arg := dir.Arguments.ForName("if"); arg != nil {
				if val, ok := fc.evaluateValue(arg.Value).(bool); ok && val {
					return false
				}
			}
		case "include":
			if arg := dir.Arguments.ForName("if"); arg != nil {
				if val, ok := fc.evaluateValue(arg.Value).(bool); ok && !val {
					return false
				}
			}
		}
	}
	return true
}

// collectArguments extracts argument values, resolving variables
func (fc *FieldCollector) collectArguments(args ast.ArgumentList) map[string]interface{} {
	result := make(map[string]interface{})

	for _, arg := range args {
		result[arg.Name] = fc.evaluateValue(arg.Value)
	}

	return result
}

// collectDirectives extracts directive instances
func (fc *FieldCollector) collectDirectives(dirs ast.DirectiveList) []*DirectiveInstance {
	result := make([]*DirectiveInstance, 0, len(dirs))

	for _, dir := range dirs {
		if dir.Name == "skip" || dir.Name == "include" {
			continue // These are handled separately
		}

		d := &DirectiveInstance{
			Name:      dir.Name,
			Arguments: make(map[string]interface{}),
		}

		for _, arg := range dir.Arguments {
			d.Arguments[arg.Name] = fc.evaluateValue(arg.Value)
		}

		result = append(result, d)
	}

	return result
}

// evaluateValue evaluates a value, resolving variables
func (fc *FieldCollector) evaluateValue(value *ast.Value) interface{} {
	if value == nil {
		return nil
	}

	switch value.Kind {
	case ast.Variable:
		if fc.variables != nil {
			return fc.variables[value.Raw]
		}
		return nil

	case ast.IntValue:
		// Parse as int64
		var i int64
		if _, err := parseValue(value.Raw, &i); err == nil {
			return i
		}
		return 0

	case ast.FloatValue:
		// Parse as float64
		var f float64
		if _, err := parseValue(value.Raw, &f); err == nil {
			return f
		}
		return 0.0

	case ast.StringValue, ast.BlockValue:
		return value.Raw

	case ast.BooleanValue:
		return value.Raw == "true"

	case ast.NullValue:
		return nil

	case ast.EnumValue:
		return value.Raw

	case ast.ListValue:
		result := make([]interface{}, 0, len(value.Children))
		for _, child := range value.Children {
			result = append(result, fc.evaluateValue(child.Value))
		}
		return result

	case ast.ObjectValue:
		result := make(map[string]interface{})
		for _, child := range value.Children {
			result[child.Name] = fc.evaluateValue(child.Value)
		}
		return result

	default:
		return value.Raw
	}
}

// parseValue parses a string value into a specific type
func parseValue(s string, dst interface{}) (interface{}, error) {
	switch v := dst.(type) {
	case *int64:
		var val int64
		for i := 0; i < len(s); i++ {
			if s[i] >= '0' && s[i] <= '9' {
				val = val*10 + int64(s[i]-'0')
			} else if i == 0 && s[i] == '-' {
				continue
			} else {
				break
			}
		}
		if len(s) > 0 && s[0] == '-' {
			val = -val
		}
		*v = val
		return val, nil

	case *float64:
		var val float64
		var decimal float64 = 1
		inDecimal := false
		for i := 0; i < len(s); i++ {
			if s[i] == '.' {
				inDecimal = true
				continue
			}
			if s[i] >= '0' && s[i] <= '9' {
				if inDecimal {
					decimal *= 10
					val = val + float64(s[i]-'0')/decimal
				} else {
					val = val*10 + float64(s[i]-'0')
				}
			}
		}
		if len(s) > 0 && s[0] == '-' {
			val = -val
		}
		*v = val
		return val, nil
	}

	return s, nil
}

// getFieldTypeName gets the return type name for a field
func (fc *FieldCollector) getFieldTypeName(parentType, fieldName string) string {
	objType, ok := fc.schema.GetType(parentType)
	if !ok {
		return ""
	}

	field, ok := objType.Fields[fieldName]
	if !ok {
		return ""
	}

	return fc.unwrapTypeName(field.Type)
}

// unwrapTypeName unwraps a TypeRef to get the underlying type name
func (fc *FieldCollector) unwrapTypeName(t *TypeRef) string {
	if t == nil {
		return ""
	}

	if t.IsList {
		return fc.unwrapTypeName(t.ListElem)
	}

	return t.Name
}

// typeApplies checks if a fragment type condition applies to a runtime type
func (fc *FieldCollector) typeApplies(fragmentType, runtimeType string) bool {
	// Exact match
	if fragmentType == runtimeType {
		return true
	}

	// Check if runtime type implements fragment type (interface)
	if objType, ok := fc.schema.GetType(runtimeType); ok {
		for _, impl := range objType.Implements {
			if impl == fragmentType {
				return true
			}
		}
	}

	return false
}

// CollectedField represents a field after collection with all merged info
type CollectedField struct {
	Field      *SelectedField
	Path       []string
	ParentType string
}

// FlattenFields flattens a selection set into a list of collected fields with paths
func (fc *FieldCollector) FlattenFields(selections *SelectionSet, parentType string, parentPath []string) []*CollectedField {
	if selections == nil {
		return nil
	}

	result := make([]*CollectedField, 0)

	for _, field := range selections.Fields {
		path := append(parentPath, field.GetName())

		cf := &CollectedField{
			Field:      field,
			Path:       path,
			ParentType: parentType,
		}
		result = append(result, cf)

		// Recursively flatten nested selections
		if field.HasSelection() {
			nestedType := fc.getFieldTypeName(parentType, field.Name)
			nested := fc.FlattenFields(field.Selections, nestedType, path)
			result = append(result, nested...)
		}
	}

	return result
}

// GetFieldsByPath retrieves fields at a specific path depth
func (fc *FieldCollector) GetFieldsByPath(selections *SelectionSet, parentType string, depth int) []*CollectedField {
	all := fc.FlattenFields(selections, parentType, nil)
	result := make([]*CollectedField, 0)

	for _, field := range all {
		if len(field.Path) == depth {
			result = append(result, field)
		}
	}

	return result
}

// GetFieldNames returns just the field names from a selection set
func GetFieldNames(selections *SelectionSet) []string {
	if selections == nil {
		return nil
	}

	names := make([]string, 0, len(selections.Fields))
	for _, field := range selections.Fields {
		names = append(names, field.Name)
	}
	return names
}

// GetFieldNamesRecursive returns all field names including nested fields
func GetFieldNamesRecursive(selections *SelectionSet, prefix string) []string {
	if selections == nil {
		return nil
	}

	names := make([]string, 0)
	for _, field := range selections.Fields {
		name := field.Name
		if prefix != "" {
			name = prefix + "." + name
		}

		names = append(names, name)

		if field.HasSelection() {
			nested := GetFieldNamesRecursive(field.Selections, name)
			names = append(names, nested...)
		}
	}
	return names
}
