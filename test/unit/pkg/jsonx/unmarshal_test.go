package jsonx

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type jsonDTO struct {
	Count int            `json:"count"`
	Flag  bool           `json:"flag"`
	Name  string         `json:"name"`
	Maybe *int           `json:"maybe"`
	Child *jsonDTO       `json:"child"`
	Items []int          `json:"items"`
	Pairs map[string]int `json:"pairs"`
	Any   any            `json:"any"`
}

func TestUnmarshalRejectsNullBeforeLosingItsType(t *testing.T) {
	for _, data := range []string{
		`null`, `{"count":null}`, `{"flag":null}`, `{"name":null}`,
		`{"child":{"count":null}}`, `{"items":[1,null]}`, `{"pairs":{"x":null}}`,
		`{"count":null,"count":1}`, `{"COUNT":null}`,
	} {
		t.Run(data, func(t *testing.T) {
			value := jsonDTO{Count: 17}
			var failure *NullError
			if err := Unmarshal([]byte(data), &value); !errors.As(err, &failure) {
				t.Fatalf("일반 값의 null을 타입 오류로 반환해야 합니다: %v", err)
			}
			if value.Count != 17 {
				t.Fatal("null을 검사하는 동안 대상 DTO를 변경하면 안 됩니다")
			}
		})
	}
}

func TestUnmarshalKeepsMissingFieldsAndNilableValues(t *testing.T) {
	for _, data := range []string{
		`{}`, `{"count":0,"flag":false,"name":""}`,
		`{"maybe":null,"child":null,"items":null,"pairs":null,"any":null}`,
		`{"maybe":0,"child":{"count":1},"items":[1],"pairs":{"x":2}}`,
		`{"unknown":{"count":null},"name":"null"}`,
		`{"any":{"count":null}}`,
	} {
		t.Run(data, func(t *testing.T) {
			var got, want jsonDTO
			if err := json.Unmarshal([]byte(data), &want); err != nil {
				t.Fatal(err)
			}
			if err := Unmarshal([]byte(data), &got); err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("누락, 실제 0과 nil 값을 보존해야 합니다: got=%+v want=%+v err=%v", got, want, err)
			}
		})
	}
}

func TestUnmarshalChecksCollectionsAndQuotedScalars(t *testing.T) {
	for _, test := range []struct {
		data   string
		target any
		reject bool
	}{
		{`null`, new(int), true},
		{`null`, new(json.Number), true},
		{`null`, new([2]int), true},
		{`[1,null]`, new([2]int), true},
		{`[1,null]`, new([1]int), false},
		{`null`, new([]int), false},
		{`null`, new(map[string]int), false},
		{`null`, new(*int), false},
		{`[null]`, new([]*int), false},
		{`{"count":"null"}`, &struct {
			Count int `json:"count,string"`
		}{}, true},
		{`{"count":"n\u0075ll"}`, &struct {
			Count int `json:"count,string"`
		}{}, true},
		{`{"count":null}`, &struct {
			Count *int `json:"count,string"`
		}{}, false},
		{`{"count":"0"}`, &struct {
			Count int `json:"count,string"`
		}{}, false},
	} {
		err := Unmarshal([]byte(test.data), test.target)
		var failure *NullError
		if errors.As(err, &failure) != test.reject || !test.reject && err != nil {
			t.Errorf("data=%s target=%T reject=%v err=%v", test.data, test.target, test.reject, err)
		}
	}
}

type embeddedA struct{ Count int }
type embeddedB struct{ Count int }

func TestUnmarshalUsesJSONFieldSelection(t *testing.T) {
	for _, test := range []struct {
		data   string
		target any
		reject bool
	}{
		{`{"Count":null}`, &struct{ embeddedA }{}, true},
		{`{"Count":null}`, &struct {
			embeddedA
			embeddedB
		}{}, false},
		{`{"Count":null}`, &struct {
			embeddedA
			Count *int
		}{}, false},
		{`{"Count":null}`, &struct {
			Count int `json:"-"`
		}{}, false},
		{`{"hidden":null}`, &struct{ hidden int }{}, false},
		{`{"Id":null}`, &struct {
			A int  `json:"ID"`
			B *int `json:"id"`
		}{}, true},
		{`{"id":null}`, &struct {
			A int  `json:"ID"`
			B *int `json:"id"`
		}{}, false},
		{`{"\u0063ount":null}`, &struct {
			Count int `json:"count"`
		}{}, true},
	} {
		err := Unmarshal([]byte(test.data), test.target)
		var failure *NullError
		if errors.As(err, &failure) != test.reject || !test.reject && err != nil {
			t.Errorf("JSON 필드 선택 규칙이 달라졌습니다: target=%T data=%s err=%v", test.target, test.data, err)
		}
	}
}

type customNull struct{ Calls int }

func (v *customNull) UnmarshalJSON([]byte) error { v.Calls++; return nil }

func TestUnmarshalLeavesCustomJSONAndInvalidInputToTheirOwners(t *testing.T) {
	var custom customNull
	if err := Unmarshal([]byte(`null`), &custom); err != nil || custom.Calls != 1 {
		t.Fatalf("사용자 정의 디코더가 null을 한 번 처리해야 합니다: %+v %v", custom, err)
	}
	var raw json.RawMessage
	if err := Unmarshal([]byte(`null`), &raw); err != nil || string(raw) != "null" {
		t.Fatalf("RawMessage는 null 원문을 보존해야 합니다: %s %v", raw, err)
	}
	for _, data := range []string{`{"count":null`, `null true`, `{"count":"wrong"}`} {
		var got, want jsonDTO
		gotErr := Unmarshal([]byte(data), &got)
		wantErr := json.Unmarshal([]byte(data), &want)
		if reflect.TypeOf(gotErr) != reflect.TypeOf(wantErr) || gotErr.Error() != wantErr.Error() {
			t.Fatalf("기존 구문과 타입 오류를 보존해야 합니다: %v != %v", gotErr, wantErr)
		}
	}
	for _, target := range []any{nil, 1, (*jsonDTO)(nil)} {
		var failure *json.InvalidUnmarshalError
		if err := Unmarshal([]byte(`null`), target); !errors.As(err, &failure) {
			t.Fatalf("잘못된 대상의 기존 오류를 보존해야 합니다: %v", err)
		}
	}
}

func FuzzUnmarshalPreservesNonNullJSON(f *testing.F) {
	for _, data := range []string{`{}`, `{"count":1}`, `{"name":"hello"}`, `{"maybe":0}`, `{"child":{"count":1}}`} {
		f.Add([]byte(data))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var got, want jsonDTO
		gotErr, wantErr := Unmarshal(data, &got), json.Unmarshal(data, &want)
		var null *NullError
		if errors.As(gotErr, &null) {
			return
		}
		if (gotErr == nil) != (wantErr == nil) || !reflect.DeepEqual(got, want) {
			t.Fatalf("null 거부 외의 JSON 동작이 달라졌습니다: got=%+v want=%+v errors=%v / %v", got, want, gotErr, wantErr)
		}
	})
}
