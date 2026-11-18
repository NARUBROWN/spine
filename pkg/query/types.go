package query

import (
	"strconv"
	"strings"
)

// Values is the legacy query lookup type retained during migration.
// Its lookup keys are determined at runtime, so detailed input schemas cannot be generated.
//
// Deprecated: Use input.Query[T] with a DTO that declares query tags.
type Values struct {
	values map[string][]string
}

// NewValues creates a query view for the legacy lookup API.
//
// Deprecated: Use a DTO with query tags and input.Query[T].
func NewValues(values map[string][]string) Values {
	return Values{values: values}
}

// Get returns the first value, or an empty string if the key or value is missing.
//
// Deprecated: Read struct fields bound with query tags directly.
func (q Values) Get(key string) string {
	if v, ok := q.values[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

// String returns the same first value as Get.
//
// Deprecated: Read struct fields bound with query tags directly.
func (q Values) String(key string) string {
	return q.Get(key)
}

// Int parses the first value as an integer, returning def if it is missing or invalid.
//
// Deprecated: Bind an integer field and use a pointer field when missing values need a default.
func (q Values) Int(key string, def int64) int64 {
	raw := q.Get(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// GetBoolByKey accepts legacy true/false, yes/no and on/off spellings.
// It returns def when the value is missing or unrecognized.
//
// Deprecated: Use a bool field or a TextUnmarshaler for the required format.
func (q Values) GetBoolByKey(key string, def bool) bool {
	raw := strings.ToLower(q.Get(key))
	switch raw {
	case "true", "1", "yes", "y", "on":
		return true
	case "false", "0", "no", "n", "off":
		return false
	default:
		return def
	}
}

// Has reports whether a key exists, including keys with empty values.
//
// Deprecated: Use pointer fields when you need to distinguish missing values.
func (q Values) Has(key string) bool {
	_, ok := q.values[key]
	return ok
}
