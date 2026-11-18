package header

import "net/http"

// Values is the legacy HTTP header lookup type retained during migration.
// Its lookup keys are determined at runtime, so detailed input schemas cannot be generated.
//
// Deprecated: Use input.Header[T] with a DTO that declares header tags.
type Values struct {
	headers http.Header
}

// NewValues creates a legacy lookup view over the supplied headers.
//
// Deprecated: Use a DTO with header tags and input.Header[T].
func NewValues(headers http.Header) Values {
	return Values{headers: headers}
}

// Get returns the first value using the same key matching rules as http.Header.
//
// Deprecated: Read struct fields bound with header tags directly.
func (h Values) Get(key string) string {
	return h.headers.Get(key)
}

// Has reports whether a key exists, including keys with empty values.
// Key matching follows http.Header.
//
// Deprecated: Use pointer fields when you need to distinguish missing values.
func (h Values) Has(key string) bool {
	_, ok := h.headers[http.CanonicalHeaderKey(key)]
	return ok
}
