package ast

type JoinType int

const (
	JoinInner JoinType = iota
	JoinLeft
	JoinRight
	JoinFull
	JoinCross
	JoinLateral
	JoinLeftLateral
)

type BinaryOp int

const (
	// Comparison operators
	OpEq BinaryOp = iota
	OpNeq
	OpLt
	OpLte
	OpGt
	OpGte

	// Logical operators
	OpAnd
	OpOr

	// Arithmetic operators
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod

	// String operators
	OpLike
	OpILike
	OpNotLike
	OpNotILike
	OpSimilarTo
	OpRegex
	OpRegexI // case insensitive regex

	// JSON operators (PostgreSQL)
	OpJSONArrow       // ->
	OpJSONArrowText   // ->>
	OpJSONPath        // #>
	OpJSONPathText    // #>>
	OpJSONContains    // @>
	OpJSONContainedBy // <@

	// Array operators
	OpArrayContains // @>
	OpArrayOverlap  // &&
	OpArrayConcat   // ||
)

type UnaryOp int

const (
	OpNot UnaryOp = iota
	OpNeg
	OpIsNull
	OpIsNotNull
	OpIsTrue
	OpIsFalse
	OpExists
	OpNotExists
)

type OrderDirection int

const (
	OrderAsc OrderDirection = iota
	OrderDesc
)

type JoinColumn struct {
	JoinType  JoinType
	TableName string
	Alias     string // Alias for the joined table/subquery
	On        string

	// LATERAL subquery options (for JoinLateral, JoinLeftLateral)
	SubqueryColumns []string // Columns to select in the subquery (e.g., ["id", "total"])
	SubqueryWhere   string   // WHERE clause inside the subquery (can reference outer table)
	SubqueryOrderBy string   // ORDER BY clause inside the subquery
	Limit           string   // LIMIT for the subquery
}
