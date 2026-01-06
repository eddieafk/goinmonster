package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/eddieafk/goinmonster/graph/marshal"
	"github.com/eddieafk/goinmonster/sql/ast"
	"github.com/eddieafk/goinmonster/sql/dialect"
	"github.com/eddieafk/goinmonster/sql/stringifiers/dialecttypes"
)

// SQLConverter converts GraphQL queries to SQL
type SQLConverter struct {
	schema     *Schema
	dialect    dialect.Dialect
	marshaler  *marshal.PostgreSQLMarshaler
	tableMap   map[string]string            // GraphQL type -> SQL table
	columnMap  map[string]map[string]string // type.field -> SQL column
	joinConfig map[string]*JoinConfig       // type.field -> join configuration
}

// JoinConfig describes how to join related types
type JoinConfig struct {
	SourceTable   string
	SourceColumn  string
	TargetTable   string
	TargetColumn  string
	JoinType      ast.JoinType
	RelationType  string // "hasOne", "hasMany", "belongsTo", "manyToMany"
	ThroughTable  string // For manyToMany
	ThroughSource string // For manyToMany
	ThroughTarget string // For manyToMany
}

// NewSQLConverter creates a new SQL converter
func NewSQLConverter(schema *Schema, d dialect.Dialect) *SQLConverter {
	return &SQLConverter{
		schema:     schema,
		dialect:    d,
		marshaler:  marshal.NewPostgreSQLMarshaler(),
		tableMap:   make(map[string]string),
		columnMap:  make(map[string]map[string]string),
		joinConfig: make(map[string]*JoinConfig),
	}
}

// MapTypeToTable maps a GraphQL type to a SQL table
func (c *SQLConverter) MapTypeToTable(typeName, tableName string) {
	c.tableMap[typeName] = tableName
}

// MapFieldToColumn maps a GraphQL field to a SQL column
func (c *SQLConverter) MapFieldToColumn(typeName, fieldName, columnName string) {
	if c.columnMap[typeName] == nil {
		c.columnMap[typeName] = make(map[string]string)
	}
	c.columnMap[typeName][fieldName] = columnName
}

// ConfigureJoin configures how to join related types
func (c *SQLConverter) ConfigureJoin(typeName, fieldName string, config *JoinConfig) {
	key := typeName + "." + fieldName
	c.joinConfig[key] = config
}

// getTableName gets the SQL table name for a GraphQL type
func (c *SQLConverter) getTableName(typeName string) string {
	if table, ok := c.tableMap[typeName]; ok {
		return table
	}
	// Default: snake_case of type name
	return toSnakeCase(typeName)
}

// getColumnName gets the SQL column name for a GraphQL field
func (c *SQLConverter) getColumnName(typeName, fieldName string) string {
	if cols, ok := c.columnMap[typeName]; ok {
		if col, ok := cols[fieldName]; ok {
			return col
		}
	}
	// Default: snake_case of field name
	return toSnakeCase(fieldName)
}

// SQLSelectResult contains the result of converting GraphQL to SQL SELECT
type SQLSelectResult struct {
	Query    string
	Params   []interface{}
	Options  dialecttypes.PostgreSQLSelectOptions
	Errors   []dialecttypes.ValidationError
	Warnings []string
}

// ConvertToSelect converts a GraphQL query to SQL SELECT
func (c *SQLConverter) ConvertToSelect(
	ctx context.Context,
	info *ResolveInfo,
) (*SQLSelectResult, error) {
	c.marshaler.Reset()

	// Determine the root type and table
	rootType := info.ReturnType
	if rootType == nil {
		return nil, fmt.Errorf("cannot determine return type")
	}

	typeName := unwrapTypeName(rootType)
	tableName := c.getTableName(typeName)

	// Build select options
	opts := dialecttypes.PostgreSQLSelectOptions{
		TableName:  c.dialect.QuoteIdentifier(tableName),
		TableAlias: strings.ToLower(typeName[:1]),
		Columns:    make([]string, 0),
		Joins:      make([]ast.JoinColumn, 0),
		Where:      make([]string, 0),
	}

	// Collect columns from selection set
	columns, joins := c.collectColumnsAndJoins(typeName, opts.TableAlias, info.Selection)
	opts.Columns = columns
	opts.Joins = joins

	// Process arguments (filter, pagination, ordering)
	if err := c.processArguments(info.Arguments, &opts); err != nil {
		return nil, err
	}

	// Build the query
	pg, ok := c.dialect.(dialect.PostgreSQLDialect)
	if !ok {
		return nil, fmt.Errorf("dialect does not support PostgreSQL SELECT building")
	}

	query, errors := pg.BuildSelect(opts)

	return &SQLSelectResult{
		Query:   query,
		Params:  c.marshaler.Params(),
		Options: opts,
		Errors:  errors,
	}, nil
}

// collectColumnsAndJoins collects SQL columns and joins from the GraphQL selection
func (c *SQLConverter) collectColumnsAndJoins(
	typeName string,
	tableAlias string,
	selections *SelectionSet,
) ([]string, []ast.JoinColumn) {
	columns := make([]string, 0)
	joins := make([]ast.JoinColumn, 0)

	if selections == nil {
		return columns, joins
	}

	for _, field := range selections.Fields {
		// Check if this is a relation field
		joinKey := typeName + "." + field.Name
		if joinCfg, ok := c.joinConfig[joinKey]; ok {
			// This is a join field
			joinAlias := field.GetName()[:1] + "_" + field.Name[:min(3, len(field.Name))]

			join := ast.JoinColumn{
				JoinType:  joinCfg.JoinType,
				TableName: c.dialect.QuoteIdentifier(joinCfg.TargetTable),
				Alias:     joinAlias,
				On: fmt.Sprintf("%s.%s = %s.%s",
					tableAlias,
					c.dialect.QuoteIdentifier(joinCfg.SourceColumn),
					joinAlias,
					c.dialect.QuoteIdentifier(joinCfg.TargetColumn),
				),
			}

			// If it's a lateral subquery for hasMany
			if joinCfg.RelationType == "hasMany" && field.HasSelection() {
				join.JoinType = ast.JoinLeftLateral

				// Get subquery columns
				subColumns := make([]string, 0)
				for _, subField := range field.Selections.Fields {
					subColumns = append(subColumns, c.getColumnName(unwrapFieldType(typeName, field.Name, c.schema), subField.Name))
				}
				join.SubqueryColumns = subColumns
				join.SubqueryWhere = fmt.Sprintf("%s = %s.%s",
					c.dialect.QuoteIdentifier(joinCfg.TargetColumn),
					tableAlias,
					c.dialect.QuoteIdentifier(joinCfg.SourceColumn),
				)

				// Check for limit argument
				if limit, ok := field.Arguments["limit"]; ok {
					join.Limit = fmt.Sprintf("%v", limit)
				}
			}

			joins = append(joins, join)

			// Add columns from the joined table
			if field.HasSelection() {
				for _, subField := range field.Selections.Fields {
					col := fmt.Sprintf("%s.%s AS %s_%s",
						joinAlias,
						c.dialect.QuoteIdentifier(c.getColumnName(unwrapFieldType(typeName, field.Name, c.schema), subField.Name)),
						joinAlias,
						subField.GetName(),
					)
					columns = append(columns, col)
				}
			}
		} else {
			// Regular scalar field
			colName := c.getColumnName(typeName, field.Name)
			alias := tableAlias + "." + c.dialect.QuoteIdentifier(colName)

			if field.Alias != "" && field.Alias != field.Name {
				alias = alias + " AS " + c.dialect.QuoteIdentifier(field.Alias)
			}

			columns = append(columns, alias)
		}
	}

	return columns, joins
}

// processArguments processes GraphQL arguments into SQL options
func (c *SQLConverter) processArguments(
	args map[string]interface{},
	opts *dialecttypes.PostgreSQLSelectOptions,
) error {
	if args == nil {
		return nil
	}

	// Handle 'where' or 'filter' argument
	if where, ok := args["where"].(map[string]interface{}); ok {
		whereBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
		if err := c.buildWhereFromFilter(where, opts.TableAlias, whereBuilder); err != nil {
			return err
		}
		if clause := whereBuilder.Build(); clause != "" {
			opts.Where = append(opts.Where, clause)
		}
	}

	if filter, ok := args["filter"].(map[string]interface{}); ok {
		whereBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
		if err := c.buildWhereFromFilter(filter, opts.TableAlias, whereBuilder); err != nil {
			return err
		}
		if clause := whereBuilder.Build(); clause != "" {
			opts.Where = append(opts.Where, clause)
		}
	}

	// Handle 'id' argument (common shortcut)
	if id, ok := args["id"]; ok {
		placeholder, _ := c.marshaler.MarshalValue(id)
		opts.Where = append(opts.Where, opts.TableAlias+".id = "+placeholder)
	}

	// Handle 'limit' and 'first' arguments
	if limit, ok := args["limit"]; ok {
		opts.Limit = fmt.Sprintf("%v", limit)
	} else if first, ok := args["first"]; ok {
		opts.Limit = fmt.Sprintf("%v", first)
	}

	// Handle 'offset' and 'skip' arguments
	if offset, ok := args["offset"]; ok {
		opts.Offset = fmt.Sprintf("%v", offset)
	} else if skip, ok := args["skip"]; ok {
		opts.Offset = fmt.Sprintf("%v", skip)
	}

	// Handle 'orderBy' argument
	if orderBy, ok := args["orderBy"].([]interface{}); ok {
		orderBuilder := marshal.NewOrderByBuilder()
		for _, o := range orderBy {
			if orderMap, ok := o.(map[string]interface{}); ok {
				if field, ok := orderMap["field"].(string); ok {
					direction := "ASC"
					if dir, ok := orderMap["direction"].(string); ok {
						direction = dir
					}
					orderBuilder.Add(opts.TableAlias+"."+c.dialect.QuoteIdentifier(toSnakeCase(field)), direction)
				}
			}
		}
		for _, col := range orderBuilder.Build() {
			opts.OrderBy = append(opts.OrderBy, dialecttypes.OrderByColumn{
				Column:    col,
				Direction: ast.OrderAsc,
			})
		}
	} else if orderBy, ok := args["orderBy"].(map[string]interface{}); ok {
		if field, ok := orderBy["field"].(string); ok {
			direction := ast.OrderAsc
			if dir, ok := orderBy["direction"].(string); ok && strings.ToUpper(dir) == "DESC" {
				direction = ast.OrderDesc
			}
			opts.OrderBy = append(opts.OrderBy, dialecttypes.OrderByColumn{
				Column:    opts.TableAlias + "." + c.dialect.QuoteIdentifier(toSnakeCase(field)),
				Direction: direction,
			})
		}
	}

	return nil
}

// buildWhereFromFilter builds WHERE clauses from a filter object
func (c *SQLConverter) buildWhereFromFilter(
	filter map[string]interface{},
	tableAlias string,
	builder *marshal.WhereClauseBuilder,
) error {
	for key, value := range filter {
		switch key {
		case "_and", "AND":
			if conditions, ok := value.([]interface{}); ok {
				for _, cond := range conditions {
					if condMap, ok := cond.(map[string]interface{}); ok {
						if err := c.buildWhereFromFilter(condMap, tableAlias, builder); err != nil {
							return err
						}
					}
				}
			}

		case "_or", "OR":
			if conditions, ok := value.([]interface{}); ok {
				orBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
				for _, cond := range conditions {
					if condMap, ok := cond.(map[string]interface{}); ok {
						subBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
						if err := c.buildWhereFromFilter(condMap, tableAlias, subBuilder); err != nil {
							return err
						}
						orBuilder.AddRaw(subBuilder.Build())
					}
				}
				builder.AddRaw(orBuilder.BuildOr())
			}

		case "_not", "NOT":
			if notFilter, ok := value.(map[string]interface{}); ok {
				subBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
				if err := c.buildWhereFromFilter(notFilter, tableAlias, subBuilder); err != nil {
					return err
				}
				builder.AddRaw("NOT (" + subBuilder.Build() + ")")
			}

		default:
			// Field condition
			column := tableAlias + "." + c.dialect.QuoteIdentifier(toSnakeCase(key))

			switch v := value.(type) {
			case map[string]interface{}:
				// Operator-based filter: {age: {_gt: 18}}
				for op, operand := range v {
					sqlOp := convertGraphQLOperator(op)
					if err := builder.AddCondition(column, sqlOp, operand); err != nil {
						return err
					}
				}

			default:
				// Direct equality: {name: "John"}
				if err := builder.AddCondition(column, "eq", value); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// convertGraphQLOperator converts GraphQL filter operators to SQL
func convertGraphQLOperator(op string) string {
	switch op {
	case "_eq", "eq":
		return "="
	case "_neq", "neq", "_ne", "ne":
		return "<>"
	case "_gt", "gt":
		return ">"
	case "_gte", "gte", "_ge", "ge":
		return ">="
	case "_lt", "lt":
		return "<"
	case "_lte", "lte", "_le", "le":
		return "<="
	case "_like", "like":
		return "like"
	case "_ilike", "ilike":
		return "ilike"
	case "_in", "in":
		return "in"
	case "_nin", "nin", "_not_in", "not_in":
		return "nin"
	case "_is_null", "is_null", "isNull":
		return "is_null"
	case "_contains", "contains":
		return "contains"
	case "_contained_by", "contained_by", "containedBy":
		return "contained_by"
	default:
		return op
	}
}

// toSnakeCase converts a string to snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			result.WriteByte(byte(r + 32))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// unwrapFieldType gets the return type of a field
func unwrapFieldType(typeName, fieldName string, schema *Schema) string {
	if objType, ok := schema.GetType(typeName); ok {
		if field, ok := objType.Fields[fieldName]; ok {
			return unwrapTypeName(field.Type)
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ConvertMutation converts a GraphQL mutation to SQL
type SQLMutationResult struct {
	Query     string
	Params    []interface{}
	Operation string // "INSERT", "UPDATE", "DELETE"
}

// ConvertToInsert converts a GraphQL mutation to SQL INSERT
func (c *SQLConverter) ConvertToInsert(
	ctx context.Context,
	typeName string,
	input map[string]interface{},
	returning []string,
) (*SQLMutationResult, error) {
	c.marshaler.Reset()

	tableName := c.getTableName(typeName)

	columns := make([]string, 0, len(input))
	values := make([]string, 0, len(input))

	for field, value := range input {
		columns = append(columns, c.dialect.QuoteIdentifier(c.getColumnName(typeName, field)))
		placeholder, err := c.marshaler.MarshalValue(value)
		if err != nil {
			return nil, err
		}
		values = append(values, placeholder)
	}

	// Build returning clause
	returningCols := make([]string, 0, len(returning))
	for _, field := range returning {
		returningCols = append(returningCols, c.dialect.QuoteIdentifier(c.getColumnName(typeName, field)))
	}

	pg, ok := c.dialect.(dialect.PostgreSQLDialect)
	if !ok {
		return nil, fmt.Errorf("dialect does not support PostgreSQL INSERT building")
	}

	opts := dialecttypes.PostgreSQLInsertOptions{
		TableName: c.dialect.QuoteIdentifier(tableName),
		Columns:   columns,
		Values:    [][]string{values},
		Returning: returningCols,
	}

	query := pg.BuildInsert(opts)

	return &SQLMutationResult{
		Query:     query,
		Params:    c.marshaler.Params(),
		Operation: "INSERT",
	}, nil
}

// ConvertToUpdate converts a GraphQL mutation to SQL UPDATE
func (c *SQLConverter) ConvertToUpdate(
	ctx context.Context,
	typeName string,
	where map[string]interface{},
	set map[string]interface{},
	returning []string,
) (*SQLMutationResult, error) {
	c.marshaler.Reset()

	tableName := c.getTableName(typeName)
	tableAlias := strings.ToLower(typeName[:1])

	// Build SET clause
	setMap := make(map[string]string)
	for field, value := range set {
		placeholder, err := c.marshaler.MarshalValue(value)
		if err != nil {
			return nil, err
		}
		setMap[c.dialect.QuoteIdentifier(c.getColumnName(typeName, field))] = placeholder
	}

	// Build WHERE clause
	whereBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
	if err := c.buildWhereFromFilter(where, tableAlias, whereBuilder); err != nil {
		return nil, err
	}

	// Build returning clause
	returningCols := make([]string, 0, len(returning))
	for _, field := range returning {
		returningCols = append(returningCols, c.dialect.QuoteIdentifier(c.getColumnName(typeName, field)))
	}

	pg, ok := c.dialect.(dialect.PostgreSQLDialect)
	if !ok {
		return nil, fmt.Errorf("dialect does not support PostgreSQL UPDATE building")
	}

	opts := dialecttypes.PostgreSQLUpdateOptions{
		TableName:  c.dialect.QuoteIdentifier(tableName),
		TableAlias: tableAlias,
		Set:        setMap,
		Where:      []string{whereBuilder.Build()},
		Returning:  returningCols,
	}

	query := pg.BuildUpdate(opts)

	return &SQLMutationResult{
		Query:     query,
		Params:    c.marshaler.Params(),
		Operation: "UPDATE",
	}, nil
}

// ConvertToDelete converts a GraphQL mutation to SQL DELETE
func (c *SQLConverter) ConvertToDelete(
	ctx context.Context,
	typeName string,
	where map[string]interface{},
	returning []string,
) (*SQLMutationResult, error) {
	c.marshaler.Reset()

	tableName := c.getTableName(typeName)
	tableAlias := strings.ToLower(typeName[:1])

	// Build WHERE clause
	whereBuilder := marshal.NewWhereClauseBuilder(c.marshaler)
	if err := c.buildWhereFromFilter(where, tableAlias, whereBuilder); err != nil {
		return nil, err
	}

	// Build returning clause
	returningCols := make([]string, 0, len(returning))
	for _, field := range returning {
		returningCols = append(returningCols, c.dialect.QuoteIdentifier(c.getColumnName(typeName, field)))
	}

	pg, ok := c.dialect.(dialect.PostgreSQLDialect)
	if !ok {
		return nil, fmt.Errorf("dialect does not support PostgreSQL DELETE building")
	}

	opts := dialecttypes.PostgreSQLDeleteOptions{
		TableName:  c.dialect.QuoteIdentifier(tableName),
		TableAlias: tableAlias,
		Where:      []string{whereBuilder.Build()},
		Returning:  returningCols,
	}

	query := pg.BuildDelete(opts)

	return &SQLMutationResult{
		Query:     query,
		Params:    c.marshaler.Params(),
		Operation: "DELETE",
	}, nil
}
