package dialecttypes

import "github.com/eddieafk/goinmonster/sql/ast"

// ValidationError represents a SQL construction error
type ValidationError struct {
	Field   string
	Message string
}

// OrderByColumn represents an ORDER BY column with direction
type OrderByColumn struct {
	Column    string
	Direction ast.OrderDirection
}

// PostgreSQLSelectOptions represents SELECT options specific to PostgreSQL
// with built-in validation to prevent contradictory SQL constructs
type PostgreSQLSelectOptions struct {
	TableName  string
	TableAlias string

	// Columns to select
	Columns []string

	// DISTINCT ON - PostgreSQL specific
	// Note: When using DistinctOn, ORDER BY must start with the same columns
	DistinctOn []string

	// JOIN configuration
	Joins []ast.JoinColumn

	// WHERE conditions (will be joined with AND)
	Where []string

	// GROUP BY columns
	// Note: All non-aggregated columns in SELECT must be in GROUP BY
	GroupBy []string

	// HAVING conditions (requires GROUP BY)
	Having []string

	// ORDER BY configuration
	OrderBy    []OrderByColumn
	NullsFirst *bool

	// Pagination
	Limit  string
	Offset string

	// FOR UPDATE - row locking
	// Note: Not compatible with certain JOIN types
	ForUpdate   bool
	ForUpdateOf []string // Specific tables to lock
}

// Validate checks for contradictory SQL constructs
func (o *PostgreSQLSelectOptions) Validate() []ValidationError {
	var errors []ValidationError

	// 1. DISTINCT ON + GROUP BY contradiction
	if len(o.DistinctOn) > 0 && len(o.GroupBy) > 0 {
		errors = append(errors, ValidationError{
			Field:   "DistinctOn/GroupBy",
			Message: "DISTINCT ON and GROUP BY should not be used together; they serve similar purposes",
		})
	}

	// 2. DISTINCT ON requires ORDER BY to start with same columns
	if len(o.DistinctOn) > 0 && len(o.OrderBy) > 0 {
		for i, col := range o.DistinctOn {
			if i >= len(o.OrderBy) || o.OrderBy[i].Column != col {
				errors = append(errors, ValidationError{
					Field:   "DistinctOn/OrderBy",
					Message: "ORDER BY must start with DISTINCT ON columns in the same order",
				})
				break
			}
		}
	}

	// 3. GROUP BY must include all non-aggregated SELECT columns
	// This is a simplified check - real implementation would need expression parsing

	// 4. FOR UPDATE incompatibility with certain JOINs
	if o.ForUpdate && len(o.ForUpdateOf) == 0 {
		for _, j := range o.Joins {
			if j.JoinType == ast.JoinLeft || j.JoinType == ast.JoinRight || j.JoinType == ast.JoinFull {
				errors = append(errors, ValidationError{
					Field:   "ForUpdate/Join",
					Message: "FOR UPDATE is not recommended with LEFT/RIGHT/FULL OUTER JOINs; consider using FOR UPDATE OF specific_table",
				})
				break
			}
		}
	}

	// 5. HAVING requires GROUP BY
	if len(o.Having) > 0 && len(o.GroupBy) == 0 {
		errors = append(errors, ValidationError{
			Field:   "Having",
			Message: "HAVING clause requires GROUP BY",
		})
	}

	// 6. LEFT JOIN with aggregate HAVING might indicate wrong JOIN type
	if len(o.Having) > 0 {
		for _, j := range o.Joins {
			if j.JoinType == ast.JoinLeft {
				errors = append(errors, ValidationError{
					Field:   "Join/Having",
					Message: "LEFT JOIN with HAVING on joined table aggregates may indicate INNER JOIN is more appropriate",
				})
				break
			}
		}
	}

	return errors
}

// OnConflictClause represents PostgreSQL ON CONFLICT clause
type OnConflictClause struct {
	Columns   []string // Conflict target columns
	DoNothing bool
	DoUpdate  *DoUpdateClause
}

// DoUpdateClause represents DO UPDATE SET clause
type DoUpdateClause struct {
	SetColumns []string
	SetValues  []string
	Where      []string
}

// PostgreSQLInsertOptions represents INSERT options for PostgreSQL
type PostgreSQLInsertOptions struct {
	TableName  string
	Columns    []string
	Values     [][]string
	OnConflict *OnConflictClause
	Returning  []string
}

// PostgreSQLUpdateOptions represents UPDATE options for PostgreSQL
type PostgreSQLUpdateOptions struct {
	TableName  string
	TableAlias string
	Set        map[string]string
	From       []ast.JoinColumn
	Where      []string
	Returning  []string
}

// PostgreSQLDeleteOptions represents DELETE options for PostgreSQL
type PostgreSQLDeleteOptions struct {
	TableName  string
	TableAlias string
	Using      []ast.JoinColumn
	Where      []string
	Returning  []string
}
