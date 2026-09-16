package header

import (
	"net/http"
	"testing"
)

func TestHasDetectsEmptyHeaderValues(t *testing.T) {
	t.Parallel()

	missing := NewValues(http.Header{})
	if missing.Has("X-Feature") {
		t.Fatal("missing header must return false")
	}

	present := NewValues(http.Header{"X-Feature": []string{"on"}})
	if !present.Has("X-Feature") {
		t.Fatal("header with a value must return true")
	}

	empty := NewValues(http.Header{"X-Feature": []string{""}})
	if !empty.Has("X-Feature") {
		t.Fatal("header with an empty value must return true")
	}

	if !empty.Has("x-feature") {
		t.Fatal("Has must follow http.Header canonical key matching")
	}
}
