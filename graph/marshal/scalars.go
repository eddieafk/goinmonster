package marshal

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Marshaler is the interface for types that can marshal/unmarshal GraphQL values
type Marshaler interface {
	MarshalGQL(w io.Writer) error
}

// Unmarshaler is the interface for types that can unmarshal from GraphQL
type Unmarshaler interface {
	UnmarshalGQL(v interface{}) error
}

// WriterFunc wraps a function as a Marshaler
type WriterFunc func(w io.Writer) error

func (f WriterFunc) MarshalGQL(w io.Writer) error {
	return f(w)
}

// MarshalString marshals a string
func MarshalString(s string) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		_, err := writeQuotedString(w, s)
		return err
	})
}

// UnmarshalString unmarshals a string
func UnmarshalString(v interface{}) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case *string:
		if v == nil {
			return "", nil
		}
		return *v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("cannot unmarshal %T as string", v)
	}
}

// MarshalInt marshals an int
func MarshalInt(i int) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, strconv.Itoa(i))
		return err
	})
}

// UnmarshalInt unmarshals an int
func UnmarshalInt(v interface{}) (int, error) {
	switch v := v.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	case json.Number:
		i, err := v.Int64()
		return int(i), err
	default:
		return 0, fmt.Errorf("cannot unmarshal %T as int", v)
	}
}

// MarshalInt64 marshals an int64
func MarshalInt64(i int64) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, strconv.FormatInt(i, 10))
		return err
	})
}

// UnmarshalInt64 unmarshals an int64
func UnmarshalInt64(v interface{}) (int64, error) {
	switch v := v.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case json.Number:
		return v.Int64()
	default:
		return 0, fmt.Errorf("cannot unmarshal %T as int64", v)
	}
}

// MarshalFloat marshals a float64
func MarshalFloat(f float64) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		_, err := io.WriteString(w, strconv.FormatFloat(f, 'f', -1, 64))
		return err
	})
}

// UnmarshalFloat unmarshals a float64
func UnmarshalFloat(v interface{}) (float64, error) {
	switch v := v.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	case json.Number:
		return v.Float64()
	default:
		return 0, fmt.Errorf("cannot unmarshal %T as float", v)
	}
}

// MarshalBoolean marshals a bool
func MarshalBoolean(b bool) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		if b {
			_, err := io.WriteString(w, "true")
			return err
		}
		_, err := io.WriteString(w, "false")
		return err
	})
}

// UnmarshalBoolean unmarshals a bool
func UnmarshalBoolean(v interface{}) (bool, error) {
	switch v := v.(type) {
	case bool:
		return v, nil
	case string:
		return v == "true" || v == "1", nil
	case int:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot unmarshal %T as bool", v)
	}
}

// MarshalID marshals an ID (string)
func MarshalID(id string) Marshaler {
	return MarshalString(id)
}

// UnmarshalID unmarshals an ID
func UnmarshalID(v interface{}) (string, error) {
	switch v := v.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("cannot unmarshal %T as ID", v)
	}
}

// MarshalTime marshals a time.Time in RFC3339 format
func MarshalTime(t time.Time) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		_, err := writeQuotedString(w, t.Format(time.RFC3339))
		return err
	})
}

// UnmarshalTime unmarshals a time.Time
func UnmarshalTime(v interface{}) (time.Time, error) {
	switch v := v.(type) {
	case string:
		return time.Parse(time.RFC3339, v)
	case time.Time:
		return v, nil
	case *time.Time:
		if v == nil {
			return time.Time{}, nil
		}
		return *v, nil
	default:
		return time.Time{}, fmt.Errorf("cannot unmarshal %T as Time", v)
	}
}

// MarshalJSON marshals arbitrary JSON
func MarshalJSON(v interface{}) Marshaler {
	return WriterFunc(func(w io.Writer) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
}

// UnmarshalJSON unmarshals JSON
func UnmarshalJSON(v interface{}) (interface{}, error) {
	switch v := v.(type) {
	case map[string]interface{}:
		return v, nil
	case []interface{}:
		return v, nil
	case string:
		var result interface{}
		err := json.Unmarshal([]byte(v), &result)
		return result, err
	default:
		return v, nil
	}
}

// MarshalMap marshals a map
func MarshalMap(m map[string]interface{}) Marshaler {
	return MarshalJSON(m)
}

// UnmarshalMap unmarshals a map
func UnmarshalMap(v interface{}) (map[string]interface{}, error) {
	switch v := v.(type) {
	case map[string]interface{}:
		return v, nil
	case string:
		var result map[string]interface{}
		err := json.Unmarshal([]byte(v), &result)
		return result, err
	default:
		return nil, fmt.Errorf("cannot unmarshal %T as map", v)
	}
}

// MarshalSlice marshals a slice
func MarshalSlice(s []interface{}) Marshaler {
	return MarshalJSON(s)
}

// writeQuotedString writes a JSON-quoted string
func writeQuotedString(w io.Writer, s string) (int, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return w.Write(b)
}

// Null represents a null value
var Null = WriterFunc(func(w io.Writer) error {
	_, err := io.WriteString(w, "null")
	return err
})

// MarshalNullable wraps a marshaler to handle nil values
func MarshalNullable(m Marshaler) Marshaler {
	if m == nil {
		return Null
	}
	return m
}
