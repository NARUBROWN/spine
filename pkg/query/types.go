package query

import (
	"strconv"
	"strings"
)

type Values struct {
	values map[string][]string
}

func NewValues(values map[string][]string) Values {
	return Values{values: values}
}

func (q Values) Get(key string) string {
	if v, ok := q.values[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

func (q Values) String(key string) string {
	if v, ok := q.values[key]; ok && len(v) > 0 {
		return v[0]
	}
	return ""
}

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

func (q Values) Has(key string) bool {
	_, ok := q.values[key]
	return ok
}
