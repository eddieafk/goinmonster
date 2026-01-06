package stringifiers

import (
	"strings"

	"github.com/eddieafk/goinmonster/sql/ast"
	"github.com/eddieafk/goinmonster/sql/dialect"
)

type StringBuilder struct {
	strings.Builder
	Dialect dialect.Dialect
}

type SelectOptions struct {
	TableName  string
	Columns    []string
	Where      []string
	Joins      []ast.JoinColumn
	DistinctOn []string
	OrderBy    []string
	Limit      string
	Offset     string
	ForUpdate  bool
	Returning  []string
	GroupBy    []string
	Having     []string
	NullsFirst *bool
}

func (s *StringBuilder) BuildSelect(opts SelectOptions) string {
	// SELECT clause
	s.WriteString("SELECT ")

	// DISTINCT ON (PostgreSQL specific)
	if s.Dialect.SupportsDistinctOn() && len(opts.DistinctOn) > 0 {
		s.WriteString("DISTINCT ON (")
		s.WriteString(strings.Join(opts.DistinctOn, ", "))
		s.WriteString(") ")
	}

	// Columns
	if len(opts.Columns) > 0 {
		s.WriteString(strings.Join(opts.Columns, ", "))
	} else {
		s.WriteString("*")
	}

	// FROM clause
	s.WriteString(" FROM ")
	if opts.TableName != "" {
		s.WriteString(opts.TableName)
	} else {
		s.WriteString("table_name")
	}

	// JOIN clause
	if s.Dialect.SupportsLiteralJoin() && len(opts.Joins) > 0 {
		for _, j := range opts.Joins {
			s.WriteString(" ")
			s.WriteString(s.Dialect.FormatJoinType(j.JoinType))
			s.WriteString(" ")
			s.WriteString(j.TableName)
			if j.On != "" {
				s.WriteString(" ON ")
				s.WriteString(j.On)
			}
		}
	}

	// WHERE clause
	if len(opts.Where) > 0 {
		s.WriteString(" WHERE ")
		s.WriteString(strings.Join(opts.Where, " AND "))
	}

	// GROUP BY clause
	if len(opts.GroupBy) > 0 {
		s.WriteString(" GROUP BY ")
		s.WriteString(strings.Join(opts.GroupBy, ", "))
	}

	// HAVING clause
	if len(opts.Having) > 0 {
		s.WriteString(" HAVING ")
		s.WriteString(strings.Join(opts.Having, " AND "))
	}

	// ORDER BY clause
	if len(opts.OrderBy) > 0 {
		s.WriteString(" ORDER BY ")
		s.WriteString(strings.Join(opts.OrderBy, ", "))

		// NULLS FIRST/LAST
		if s.Dialect.SupportsNullsFirstLast() && opts.NullsFirst != nil {
			s.WriteString(" ")
			s.WriteString(s.Dialect.FormatNullsOrder(opts.NullsFirst))
		}
	}

	// LIMIT/OFFSET clause
	if s.Dialect.SupportsLimitOffset() {
		if opts.Limit != "" {
			s.WriteString(" LIMIT ")
			s.WriteString(opts.Limit)
		}
		if opts.Offset != "" {
			s.WriteString(" OFFSET ")
			s.WriteString(opts.Offset)
		}
	}

	// FOR UPDATE clause
	if s.Dialect.SupportsForUpdate() && opts.ForUpdate {
		s.WriteString(" FOR UPDATE")
	}

	// RETURNING clause (typically for INSERT/UPDATE/DELETE, but can be checked here)
	if s.Dialect.SupportReturning() && len(opts.Returning) > 0 {
		s.WriteString(" RETURNING ")
		s.WriteString(strings.Join(opts.Returning, ", "))
	}

	return s.String()
}
