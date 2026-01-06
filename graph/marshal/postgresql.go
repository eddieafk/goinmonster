package marshal

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// PostgreSQLMarshaler handles PostgreSQL-specific type conversions
type PostgreSQLMarshaler struct {
	// Placeholder counter for parameterized queries
	placeholderCounter int
	// Collected parameters
	params []interface{}
}

// NewPostgreSQLMarshaler creates a new PostgreSQL marshaler
func NewPostgreSQLMarshaler() *PostgreSQLMarshaler {
	return &PostgreSQLMarshaler{
		placeholderCounter: 0,
		params:             make([]interface{}, 0),
	}
}

// Reset resets the marshaler for a new query
func (m *PostgreSQLMarshaler) Reset() {
	m.placeholderCounter = 0
	m.params = make([]interface{}, 0)
}

// Params returns the collected parameters
func (m *PostgreSQLMarshaler) Params() []interface{} {
	return m.params
}

// NextPlaceholder returns the next placeholder string ($1, $2, etc.)
func (m *PostgreSQLMarshaler) NextPlaceholder() string {
	m.placeholderCounter++
	return "$" + strconv.Itoa(m.placeholderCounter)
}

// AddParam adds a parameter and returns its placeholder
func (m *PostgreSQLMarshaler) AddParam(v interface{}) string {
	m.params = append(m.params, v)
	return m.NextPlaceholder()
}

// MarshalValue converts a GraphQL value to SQL placeholder and parameter
func (m *PostgreSQLMarshaler) MarshalValue(v interface{}) (string, error) {
	if v == nil {
		return "NULL", nil
	}

	switch val := v.(type) {
	case string:
		return m.AddParam(val), nil

	case int, int8, int16, int32, int64:
		return m.AddParam(val), nil

	case uint, uint8, uint16, uint32, uint64:
		return m.AddParam(val), nil

	case float32, float64:
		return m.AddParam(val), nil

	case bool:
		return m.AddParam(val), nil

	case time.Time:
		return m.AddParam(val), nil

	case []string:
		// PostgreSQL array
		return m.AddParam(val), nil

	case []int:
		return m.AddParam(val), nil

	case []interface{}:
		// Handle as JSON array or PostgreSQL array
		return m.AddParam(val), nil

	case map[string]interface{}:
		// Handle as JSONB
		return m.AddParam(val), nil

	default:
		return m.AddParam(v), nil
	}
}

// MarshalLiteral converts a value to SQL literal (not parameterized)
func (m *PostgreSQLMarshaler) MarshalLiteral(v interface{}) (string, error) {
	if v == nil {
		return "NULL", nil
	}

	switch val := v.(type) {
	case string:
		return m.QuoteString(val), nil

	case int:
		return strconv.Itoa(val), nil
	case int64:
		return strconv.FormatInt(val, 10), nil
	case int32:
		return strconv.FormatInt(int64(val), 10), nil

	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32), nil

	case bool:
		if val {
			return "TRUE", nil
		}
		return "FALSE", nil

	case time.Time:
		return m.QuoteString(val.Format(time.RFC3339)), nil

	case []string:
		return m.MarshalStringArray(val), nil

	case []int:
		return m.MarshalIntArray(val), nil

	case map[string]interface{}:
		return m.MarshalJSONB(val)

	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// QuoteString escapes and quotes a string for PostgreSQL
func (m *PostgreSQLMarshaler) QuoteString(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}

// QuoteIdentifier quotes an identifier for PostgreSQL
func (m *PostgreSQLMarshaler) QuoteIdentifier(s string) string {
	escaped := strings.ReplaceAll(s, `"`, `""`)
	return `"` + escaped + `"`
}

// MarshalStringArray marshals a string array
func (m *PostgreSQLMarshaler) MarshalStringArray(arr []string) string {
	if len(arr) == 0 {
		return "ARRAY[]::text[]"
	}

	parts := make([]string, len(arr))
	for i, s := range arr {
		parts[i] = m.QuoteString(s)
	}

	return "ARRAY[" + strings.Join(parts, ", ") + "]"
}

// MarshalIntArray marshals an int array
func (m *PostgreSQLMarshaler) MarshalIntArray(arr []int) string {
	if len(arr) == 0 {
		return "ARRAY[]::integer[]"
	}

	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = strconv.Itoa(v)
	}

	return "ARRAY[" + strings.Join(parts, ", ") + "]"
}

// MarshalJSONB marshals a map as JSONB
func (m *PostgreSQLMarshaler) MarshalJSONB(v map[string]interface{}) (string, error) {
	// Simple JSON marshaling
	var sb strings.Builder
	sb.WriteString("'")
	if err := m.writeJSON(&sb, v); err != nil {
		return "", err
	}
	sb.WriteString("'::jsonb")
	return sb.String(), nil
}

func (m *PostgreSQLMarshaler) writeJSON(w io.Writer, v interface{}) error {
	switch val := v.(type) {
	case nil:
		_, err := io.WriteString(w, "null")
		return err

	case bool:
		if val {
			_, err := io.WriteString(w, "true")
			return err
		}
		_, err := io.WriteString(w, "false")
		return err

	case string:
		_, err := io.WriteString(w, `"`+strings.ReplaceAll(val, `"`, `\"`)+`"`)
		return err

	case int:
		_, err := io.WriteString(w, strconv.Itoa(val))
		return err

	case int64:
		_, err := io.WriteString(w, strconv.FormatInt(val, 10))
		return err

	case float64:
		_, err := io.WriteString(w, strconv.FormatFloat(val, 'f', -1, 64))
		return err

	case []interface{}:
		_, err := io.WriteString(w, "[")
		if err != nil {
			return err
		}
		for i, item := range val {
			if i > 0 {
				_, err = io.WriteString(w, ",")
				if err != nil {
					return err
				}
			}
			if err := m.writeJSON(w, item); err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, "]")
		return err

	case map[string]interface{}:
		_, err := io.WriteString(w, "{")
		if err != nil {
			return err
		}
		first := true
		for k, v := range val {
			if !first {
				_, err = io.WriteString(w, ",")
				if err != nil {
					return err
				}
			}
			first = false
			_, err = io.WriteString(w, `"`+k+`":`)
			if err != nil {
				return err
			}
			if err := m.writeJSON(w, v); err != nil {
				return err
			}
		}
		_, err = io.WriteString(w, "}")
		return err

	default:
		_, err := fmt.Fprintf(w, "%v", v)
		return err
	}
}

// WhereClauseBuilder helps build WHERE clauses from GraphQL filters
type WhereClauseBuilder struct {
	marshaler *PostgreSQLMarshaler
	clauses   []string
}

// NewWhereClauseBuilder creates a new WHERE clause builder
func NewWhereClauseBuilder(marshaler *PostgreSQLMarshaler) *WhereClauseBuilder {
	return &WhereClauseBuilder{
		marshaler: marshaler,
		clauses:   make([]string, 0),
	}
}

// AddCondition adds a condition to the WHERE clause
func (b *WhereClauseBuilder) AddCondition(column, op string, value interface{}) error {
	placeholder, err := b.marshaler.MarshalValue(value)
	if err != nil {
		return err
	}

	var condition string
	switch op {
	case "eq", "=":
		condition = column + " = " + placeholder
	case "neq", "!=", "<>":
		condition = column + " <> " + placeholder
	case "gt", ">":
		condition = column + " > " + placeholder
	case "gte", ">=":
		condition = column + " >= " + placeholder
	case "lt", "<":
		condition = column + " < " + placeholder
	case "lte", "<=":
		condition = column + " <= " + placeholder
	case "like":
		condition = column + " LIKE " + placeholder
	case "ilike":
		condition = column + " ILIKE " + placeholder
	case "in":
		condition = column + " = ANY(" + placeholder + ")"
	case "nin", "not_in":
		condition = column + " <> ALL(" + placeholder + ")"
	case "is_null":
		if val, ok := value.(bool); ok && val {
			condition = column + " IS NULL"
		} else {
			condition = column + " IS NOT NULL"
		}
	case "contains":
		condition = column + " @> " + placeholder
	case "contained_by":
		condition = column + " <@ " + placeholder
	default:
		condition = column + " " + op + " " + placeholder
	}

	b.clauses = append(b.clauses, condition)
	return nil
}

// AddRaw adds a raw condition
func (b *WhereClauseBuilder) AddRaw(condition string) {
	b.clauses = append(b.clauses, condition)
}

// Build returns the WHERE clause
func (b *WhereClauseBuilder) Build() string {
	if len(b.clauses) == 0 {
		return ""
	}
	return strings.Join(b.clauses, " AND ")
}

// BuildOr returns the WHERE clause with OR
func (b *WhereClauseBuilder) BuildOr() string {
	if len(b.clauses) == 0 {
		return ""
	}
	return "(" + strings.Join(b.clauses, " OR ") + ")"
}

// OrderByBuilder helps build ORDER BY clauses
type OrderByBuilder struct {
	columns []string
}

// NewOrderByBuilder creates a new ORDER BY builder
func NewOrderByBuilder() *OrderByBuilder {
	return &OrderByBuilder{
		columns: make([]string, 0),
	}
}

// Add adds a column to ORDER BY
func (b *OrderByBuilder) Add(column string, direction string) {
	dir := "ASC"
	if strings.ToUpper(direction) == "DESC" {
		dir = "DESC"
	}
	b.columns = append(b.columns, column+" "+dir)
}

// AddWithNulls adds a column with NULLS handling
func (b *OrderByBuilder) AddWithNulls(column, direction string, nullsFirst bool) {
	dir := "ASC"
	if strings.ToUpper(direction) == "DESC" {
		dir = "DESC"
	}
	nulls := "NULLS LAST"
	if nullsFirst {
		nulls = "NULLS FIRST"
	}
	b.columns = append(b.columns, column+" "+dir+" "+nulls)
}

// Build returns the ORDER BY columns
func (b *OrderByBuilder) Build() []string {
	return b.columns
}

// BuildString returns the ORDER BY clause as a string
func (b *OrderByBuilder) BuildString() string {
	return strings.Join(b.columns, ", ")
}
