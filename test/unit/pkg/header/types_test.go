package header

import (
	"net/http"
	"testing"
)

func TestLegacyHeadersPreserveFirstValueAndCanonicalPresence(t *testing.T) {
	values := NewValues(http.Header{"X-Feature": {"first", "second"}, "X-Empty": {""}})
	if values.Get("x-feature") != "first" || !values.Has("x-feature") || !values.Has("X-Empty") || values.Get("X-Empty") != "" || values.Has("missing") {
		t.Fatal("기존 헤더 조회의 첫 번째 값과 빈 헤더 구분을 유지해야 합니다")
	}
	var zero Values
	if zero.Get("X-Feature") != "" || zero.Has("X-Feature") {
		t.Fatal("초깃값이 없는 기존 헤더 조회는 비어 있어야 합니다")
	}
}
