package dialects

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eddieafk/goinmonster/sql/ast"
	"github.com/eddieafk/goinmonster/sql/stringifiers/dialecttypes"
)

type PostgreSQL struct{}

func (d PostgreSQL) Name() string { return "postgresql" }
func (d PostgreSQL) QuoteIdentifier(identifier string) string {
	return `"` + d.EscapeIdentifier(identifier) + `"`
}
func (d PostgreSQL) QuoteString(value string) string {
	return `'` + d.EscapeString(value) + `'`
}
func (d PostgreSQL) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}
func (d PostgreSQL) SupportReturning() bool        { return true }
func (d PostgreSQL) SupportsUpsert() bool          { return true }
func (d PostgreSQL) SupportsOnConflict() bool      { return true }
func (d PostgreSQL) SupportsCTE() bool             { return true }
func (d PostgreSQL) SupportsRecursiveCTE() bool    { return true }
func (d PostgreSQL) SupportsWindowFunctions() bool { return true }
func (d PostgreSQL) SupportsJSON() bool            { return true }
func (d PostgreSQL) SupportsArray() bool           { return true }
func (d PostgreSQL) SupportsLiteralJoin() bool     { return true }
func (d PostgreSQL) SupportsDistinctOn() bool      { return true }
func (d PostgreSQL) SupportsLimitOffset() bool     { return true }
func (d PostgreSQL) SupportsNullsFirstLast() bool  { return true }
func (d PostgreSQL) SupportsForUpdate() bool       { return true }
func (d PostgreSQL) SupportsMaterializedCTE() bool { return true }
func (d PostgreSQL) SupportsFullOuterJoin() bool   { return true }

/*
* ========================================================================
*                BUILDERS
* ========================================================================
 */

// BuildSelect builds a PostgreSQL SELECT statement with validation
func (d PostgreSQL) BuildSelect(opts dialecttypes.PostgreSQLSelectOptions) (string, []dialecttypes.ValidationError) {
	// Validate options first
	errors := opts.Validate()

	var sb strings.Builder

	// SELECT clause
	sb.WriteString("SELECT ")

	// DISTINCT ON (PostgreSQL specific)
	if len(opts.DistinctOn) > 0 {
		sb.WriteString("DISTINCT ON (")
		sb.WriteString(strings.Join(opts.DistinctOn, ", "))
		sb.WriteString(") ")
	}

	// Columns
	if len(opts.Columns) > 0 {
		sb.WriteString(strings.Join(opts.Columns, ", "))
	} else {
		sb.WriteString("*")
	}

	// FROM clause
	sb.WriteString("\nFROM ")
	sb.WriteString(opts.TableName)
	if opts.TableAlias != "" {
		sb.WriteString(" ")
		sb.WriteString(opts.TableAlias)
	}

	// JOIN clauses
	for _, j := range opts.Joins {
		sb.WriteString("\n")
		sb.WriteString(d.FormatJoinType(j.JoinType))
		sb.WriteString(" ")

		// Check if this is a LATERAL subquery (has subquery options)
		isLateralSubquery := j.Limit != "" || len(j.SubqueryColumns) > 0 || j.SubqueryWhere != "" || j.SubqueryOrderBy != ""

		if isLateralSubquery {
			sb.WriteString("(\n    SELECT ")
			if len(j.SubqueryColumns) > 0 {
				sb.WriteString(strings.Join(j.SubqueryColumns, ", "))
			} else {
				sb.WriteString("*")
			}
			sb.WriteString("\n    FROM ")
			sb.WriteString(j.TableName)
			if j.SubqueryWhere != "" {
				sb.WriteString("\n    WHERE ")
				sb.WriteString(j.SubqueryWhere)
			}
			if j.SubqueryOrderBy != "" {
				sb.WriteString("\n    ORDER BY ")
				sb.WriteString(j.SubqueryOrderBy)
			}
			if j.Limit != "" {
				sb.WriteString("\n    LIMIT ")
				sb.WriteString(j.Limit)
			}
			sb.WriteString("\n)")
			if j.Alias != "" {
				sb.WriteString(" ")
				sb.WriteString(j.Alias)
			}
		} else {
			sb.WriteString(j.TableName)
			if j.Alias != "" {
				sb.WriteString(" ")
				sb.WriteString(j.Alias)
			}
		}

		if j.On != "" {
			sb.WriteString(" ON ")
			sb.WriteString(j.On)
		}
	}

	// WHERE clause
	if len(opts.Where) > 0 {
		sb.WriteString("\nWHERE ")
		sb.WriteString(strings.Join(opts.Where, "\n  AND "))
	}

	// GROUP BY clause
	if len(opts.GroupBy) > 0 {
		sb.WriteString("\nGROUP BY ")
		sb.WriteString(strings.Join(opts.GroupBy, ", "))
	}

	// HAVING clause
	if len(opts.Having) > 0 {
		sb.WriteString("\nHAVING ")
		sb.WriteString(strings.Join(opts.Having, " AND "))
	}

	// ORDER BY clause
	if len(opts.OrderBy) > 0 {
		sb.WriteString("\nORDER BY ")
		orderParts := make([]string, len(opts.OrderBy))
		for i, o := range opts.OrderBy {
			orderParts[i] = o.Column + " " + d.FormatOrderDirection(o.Direction)
		}
		sb.WriteString(strings.Join(orderParts, ", "))

		// NULLS FIRST/LAST
		if opts.NullsFirst != nil {
			sb.WriteString(" ")
			sb.WriteString(d.FormatNullsOrder(opts.NullsFirst))
		}
	}

	// LIMIT/OFFSET clause
	if opts.Limit != "" {
		sb.WriteString("\nLIMIT ")
		sb.WriteString(opts.Limit)
	}
	if opts.Offset != "" {
		sb.WriteString(" OFFSET ")
		sb.WriteString(opts.Offset)
	}

	// FOR UPDATE clause
	if opts.ForUpdate {
		sb.WriteString("\nFOR UPDATE")
		if len(opts.ForUpdateOf) > 0 {
			sb.WriteString(" OF ")
			sb.WriteString(strings.Join(opts.ForUpdateOf, ", "))
		}
	}

	return sb.String(), errors
}

// BuildInsert builds a PostgreSQL INSERT statement
func (d PostgreSQL) BuildInsert(opts dialecttypes.PostgreSQLInsertOptions) string {
	var sb strings.Builder

	sb.WriteString("INSERT INTO ")
	sb.WriteString(opts.TableName)

	// Columns
	if len(opts.Columns) > 0 {
		sb.WriteString(" (")
		sb.WriteString(strings.Join(opts.Columns, ", "))
		sb.WriteString(")")
	}

	// VALUES
	sb.WriteString("\nVALUES ")
	valueParts := make([]string, len(opts.Values))
	for i, row := range opts.Values {
		valueParts[i] = "(" + strings.Join(row, ", ") + ")"
	}
	sb.WriteString(strings.Join(valueParts, ", "))

	// ON CONFLICT
	if opts.OnConflict != nil {
		sb.WriteString("\nON CONFLICT ")
		if len(opts.OnConflict.Columns) > 0 {
			sb.WriteString("(")
			sb.WriteString(strings.Join(opts.OnConflict.Columns, ", "))
			sb.WriteString(") ")
		}
		if opts.OnConflict.DoNothing {
			sb.WriteString("DO NOTHING")
		} else if opts.OnConflict.DoUpdate != nil {
			sb.WriteString("DO UPDATE SET ")
			setParts := make([]string, len(opts.OnConflict.DoUpdate.SetColumns))
			for i, col := range opts.OnConflict.DoUpdate.SetColumns {
				setParts[i] = fmt.Sprintf("%s = %s", col, opts.OnConflict.DoUpdate.SetValues[i])
			}
			sb.WriteString(strings.Join(setParts, ", "))
			if len(opts.OnConflict.DoUpdate.Where) > 0 {
				sb.WriteString(" WHERE ")
				sb.WriteString(strings.Join(opts.OnConflict.DoUpdate.Where, " AND "))
			}
		}
	}

	// RETURNING
	if len(opts.Returning) > 0 {
		sb.WriteString("\nRETURNING ")
		sb.WriteString(strings.Join(opts.Returning, ", "))
	}

	return sb.String()
}

// BuildUpdate builds a PostgreSQL UPDATE statement
func (d PostgreSQL) BuildUpdate(opts dialecttypes.PostgreSQLUpdateOptions) string {
	var sb strings.Builder

	sb.WriteString("UPDATE ")
	sb.WriteString(opts.TableName)
	if opts.TableAlias != "" {
		sb.WriteString(" ")
		sb.WriteString(opts.TableAlias)
	}

	// SET clause
	sb.WriteString("\nSET ")
	setParts := make([]string, 0, len(opts.Set))
	for col, val := range opts.Set {
		setParts = append(setParts, fmt.Sprintf("%s = %s", col, val))
	}
	sb.WriteString(strings.Join(setParts, ", "))

	// FROM clause (PostgreSQL specific for JOINs in UPDATE)
	if len(opts.From) > 0 {
		sb.WriteString("\nFROM ")
		for i, j := range opts.From {
			if i > 0 {
				sb.WriteString("\n")
				sb.WriteString(d.FormatJoinType(j.JoinType))
				sb.WriteString(" ")
			}
			sb.WriteString(j.TableName)
			if j.On != "" {
				sb.WriteString(" ON ")
				sb.WriteString(j.On)
			}
		}
	}

	// WHERE clause
	if len(opts.Where) > 0 {
		sb.WriteString("\nWHERE ")
		sb.WriteString(strings.Join(opts.Where, "\n  AND "))
	}

	// RETURNING
	if len(opts.Returning) > 0 {
		sb.WriteString("\nRETURNING ")
		sb.WriteString(strings.Join(opts.Returning, ", "))
	}

	return sb.String()
}

// BuildDelete builds a PostgreSQL DELETE statement
func (d PostgreSQL) BuildDelete(opts dialecttypes.PostgreSQLDeleteOptions) string {
	var sb strings.Builder

	sb.WriteString("DELETE FROM ")
	sb.WriteString(opts.TableName)
	if opts.TableAlias != "" {
		sb.WriteString(" ")
		sb.WriteString(opts.TableAlias)
	}

	// USING clause (PostgreSQL specific for JOINs in DELETE)
	if len(opts.Using) > 0 {
		sb.WriteString("\nUSING ")
		for i, j := range opts.Using {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(j.TableName)
		}
	}

	// WHERE clause
	if len(opts.Where) > 0 {
		sb.WriteString("\nWHERE ")
		sb.WriteString(strings.Join(opts.Where, "\n  AND "))
	}

	// RETURNING
	if len(opts.Returning) > 0 {
		sb.WriteString("\nRETURNING ")
		sb.WriteString(strings.Join(opts.Returning, ", "))
	}

	return sb.String()
}

/*
* ========================================================================
*                FORMATTERS
* ========================================================================
 */

func (d PostgreSQL) FormatLimitOffset(limit, offset ast.Expression) string {
	// Placeholder; real implementation is in "stringifier.go"
	return ""
}
func (d PostgreSQL) FormatJoinType(joinType ast.JoinType) string {
	switch joinType {
	case ast.JoinInner:
		return "INNER JOIN"
	case ast.JoinLeft:
		return "LEFT JOIN"
	case ast.JoinRight:
		return "RIGHT JOIN"
	case ast.JoinFull:
		return "FULL JOIN"
	case ast.JoinCross:
		return "CROSS JOIN"
	case ast.JoinLateral:
		return "CROSS JOIN LATERAL"
	case ast.JoinLeftLateral:
		return "LEFT JOIN LATERAL"
	default:
		return ""
	}
}

func (d PostgreSQL) FormatOrderDirection(dir ast.OrderDirection) string {
	if dir == ast.OrderAsc {
		return "ASC"
	}
	return "DESC"
}

func (d PostgreSQL) FormatNullsOrder(nullsFirst *bool) string {
	if nullsFirst == nil {
		return ""
	}
	if *nullsFirst {
		return "NULLS FIRST"
	}
	return "NULLS LAST"
}

func (d PostgreSQL) FormatBinaryOp(op ast.BinaryOp) string {
	switch op {
	case ast.OpEq:
		return "="
	case ast.OpNeq:
		return "<>"
	case ast.OpLt:
		return "<"
	case ast.OpLte:
		return "<="
	case ast.OpGt:
		return ">"
	case ast.OpGte:
		return ">="
	case ast.OpAnd:
		return "AND"
	case ast.OpOr:
		return "OR"
	case ast.OpAdd:
		return "+"
	case ast.OpSub:
		return "-"
	case ast.OpMul:
		return "*"
	case ast.OpDiv:
		return "/"
	case ast.OpMod:
		return "%"
	case ast.OpLike:
		return "LIKE"
	case ast.OpILike:
		return "ILIKE"
	case ast.OpNotLike:
		return "NOT LIKE"
	case ast.OpNotILike:
		return "NOT ILIKE"
	case ast.OpSimilarTo:
		return "SIMILAR TO"
	case ast.OpRegex:
		return "~"
	case ast.OpRegexI:
		return "~*"
	case ast.OpJSONArrow:
		return "->"
	case ast.OpJSONArrowText:
		return "->>"
	case ast.OpJSONPath:
		return "#>"
	case ast.OpJSONPathText:
		return "#>>"
	case ast.OpJSONContains:
		return "@>"
	case ast.OpJSONContainedBy:
		return "<@"
	case ast.OpArrayContains:
		return "@>"
	case ast.OpArrayOverlap:
		return "&&"
	case ast.OpArrayConcat:
		return "||"
	default:
		return ""
	}
}

func (d PostgreSQL) FormatUnaryOp(op ast.UnaryOp, prefix bool) string {
	switch op {
	case ast.OpNot:
		return "NOT"
	case ast.OpNeg:
		return "-"
	case ast.OpIsNull:
		return "IS NULL"
	case ast.OpIsNotNull:
		return "IS NOT NULL"
	case ast.OpIsTrue:
		return "IS TRUE"
	case ast.OpIsFalse:
		return "IS FALSE"
	case ast.OpExists:
		return "EXISTS"
	case ast.OpNotExists:
		return "NOT EXISTS"
	default:
		return ""
	}
}

func (d PostgreSQL) FormatBoolLiteral(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

func (d PostgreSQL) FormatCast(typeName string) string {
	return typeName
}

func (d PostgreSQL) EscapeString(value string) string {
	return strings.ReplaceAll(value, `'`, `''`)
}

func (d PostgreSQL) EscapeIdentifier(identifier string) string {
	return strings.ReplaceAll(identifier, `"`, `""`)
}
