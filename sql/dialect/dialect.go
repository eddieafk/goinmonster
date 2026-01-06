package dialect

import (
	"github.com/eddieafk/goinmonster/sql/ast"
	"github.com/eddieafk/goinmonster/sql/stringifiers/dialects"
	"github.com/eddieafk/goinmonster/sql/stringifiers/dialecttypes"
)

type Dialect interface {
	Name() string

	QuoteIdentifier(identifier string) string
	QuoteString(value string) string

	Placeholder(n int) string

	// Feature support flags
	SupportReturning() bool
	SupportsUpsert() bool
	SupportsOnConflict() bool
	SupportsCTE() bool
	SupportsRecursiveCTE() bool
	SupportsWindowFunctions() bool
	SupportsJSON() bool
	SupportsArray() bool
	SupportsLiteralJoin() bool
	SupportsDistinctOn() bool
	SupportsLimitOffset() bool
	SupportsNullsFirstLast() bool
	SupportsForUpdate() bool
	SupportsMaterializedCTE() bool
	SupportsFullOuterJoin() bool

	// Formatters
	FormatLimitOffset(limit, offset ast.Expression) string
	FormatJoinType(joinType ast.JoinType) string
	FormatOrderDirection(dir ast.OrderDirection) string
	FormatNullsOrder(nullsFirst *bool) string
	FormatBinaryOp(op ast.BinaryOp) string
	FormatUnaryOp(op ast.UnaryOp, prefix bool) string
	FormatBoolLiteral(b bool) string
	FormatCast(typeName string) string

	// Escape functions
	EscapeString(value string) string
	EscapeIdentifier(identifier string) string
}

// PostgreSQLDialect extends Dialect with PostgreSQL-specific builders
type PostgreSQLDialect interface {
	Dialect
	BuildSelect(opts dialecttypes.PostgreSQLSelectOptions) (string, []dialecttypes.ValidationError)
	BuildInsert(opts dialecttypes.PostgreSQLInsertOptions) string
	BuildUpdate(opts dialecttypes.PostgreSQLUpdateOptions) string
	BuildDelete(opts dialecttypes.PostgreSQLDeleteOptions) string
}

var (
	PostgreSQL PostgreSQLDialect = dialects.PostgreSQL{}
)
