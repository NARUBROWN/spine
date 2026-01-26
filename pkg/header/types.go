package header

import "net/http"

// Values represent the HTTP header type
type Values struct {
	headers http.Header
}

// NewValues creates a new Values instance with headers
func NewValues(headers http.Header) Values {
	return Values{headers: headers}
}

// Get returns Header value using key
func (h Values) Get(key string) string {
	return h.headers.Get(key)
}

// Has checks the key is existing
func (h Values) Has(key string) bool {
	return h.headers.Get(key) != ""
}
