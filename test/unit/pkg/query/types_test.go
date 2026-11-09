package query

import "testing"

func TestLegacyValuesPreserveFirstValueAndPresence(t *testing.T) {
	values := NewValues(map[string][]string{"tags": {"a,b", "c"}, "empty": {""}, "none": {}})
	if values.Get("tags") != "a,b" || values.String("tags") != "a,b" {
		t.Fatal("기존 조회는 첫 번째 값과 쉼표를 그대로 유지해야 합니다")
	}
	if !values.Has("empty") || !values.Has("none") || values.Has("missing") || values.Get("missing") != "" {
		t.Fatal("빈 값과 누락의 기존 구분을 유지해야 합니다")
	}
	var zero Values
	if zero.Get("key") != "" || zero.Has("key") || zero.Int("key", 7) != 7 || !zero.GetBoolByKey("key", true) {
		t.Fatal("초깃값이 없는 기존 조회도 기본값을 반환해야 합니다")
	}
}

func TestLegacyIntPreservesFallbackAndSignedValues(t *testing.T) {
	for _, test := range []struct {
		raw  string
		want int64
	}{{"", 7}, {"bad", 7}, {"9223372036854775808", 7}, {" 3", 7}, {"0", 0}, {"-2", -2}, {"3", 3}} {
		values := NewValues(map[string][]string{"count": {test.raw}})
		if got := values.Int("count", 7); got != test.want {
			t.Fatalf("Int(%q)=%d want=%d", test.raw, got, test.want)
		}
	}
}

func TestLegacyBoolPreservesAcceptedSpellingsAndFallback(t *testing.T) {
	for _, raw := range []string{"true", "1", "yes", "y", "on", "YeS", "ON"} {
		if !NewValues(map[string][]string{"flag": {raw}}).GetBoolByKey("flag", false) {
			t.Fatalf("기존 true 표기: %q", raw)
		}
	}
	for _, raw := range []string{"false", "0", "no", "n", "off", "OFF"} {
		if NewValues(map[string][]string{"flag": {raw}}).GetBoolByKey("flag", true) {
			t.Fatalf("기존 false 표기: %q", raw)
		}
	}
	for _, fallback := range []bool{false, true} {
		if got := NewValues(map[string][]string{"flag": {"unknown"}}).GetBoolByKey("flag", fallback); got != fallback {
			t.Fatalf("알 수 없는 값의 기본값: %v", got)
		}
	}
}
