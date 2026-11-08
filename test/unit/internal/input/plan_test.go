package input

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	markers "github.com/NARUBROWN/spine/pkg/input"
	"os"
	"reflect"
	"testing"
)

func TestWrappedInputNamesAndPayload(t *testing.T) {
	for field, want := range map[string]string{"Name": "name", "ID": "id", "UserID": "userId", "HTTPCode": "httpCode", "IPAddress": "ipAddress", "UserHTTPCode": "userHttpCode"} {
		if got := LowerCamel(field); got != want {
			t.Errorf("%s = %s, want %s", field, got, want)
		}
	}
	type dto struct {
		ID       int
		UserID   string
		HTTPCode string
		Alias    string `query:"custom"`
		Hidden   string `query:"-"`
	}
	plan, ok, err := Inspect(reflect.TypeFor[markers.Query[dto]]())
	if !ok || err != nil {
		t.Fatal(plan, err)
	}
	var got []string
	for _, field := range plan.Fields {
		got = append(got, field.Name)
	}
	if !reflect.DeepEqual(got, []string{"id", "userId", "httpCode", "custom"}) {
		t.Fatal(got)
	}
	if plan.Payload != reflect.TypeFor[dto]() {
		t.Fatal(plan.Payload)
	}
	if _, _, err := Inspect(reflect.TypeFor[markers.Body[*dto]]()); err == nil {
		t.Fatal("pointer payload accepted")
	}
	if plan, ok, err := Inspect(reflect.TypeFor[markers.Path[int]]()); !ok || err != nil || !plan.Scalar {
		t.Fatal(plan, err)
	}
}

func TestSharedInputNameConformance(t *testing.T) {
	data, err := os.ReadFile("../../pkg/input/testdata/conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != "4bbe6523c7a8276ba1df20f02717c392c0181001b5c2903da8381eb0c212c294" {
		t.Fatal("update the shared core and CLI conformance fixture together")
	}
	var fixture struct {
		Fields []struct {
			Go   string `json:"go"`
			Wire string `json:"wire"`
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, field := range fixture.Fields {
		if got := LowerCamel(field.Go); got != field.Wire {
			t.Errorf("%s: %s != %s", field.Go, got, field.Wire)
		}
	}
}

func TestScalarPathBinding(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  any
		raw     string
		want    any
		invalid bool
	}{
		{"string", new(string), "hello", "hello", false},
		{"bool", new(bool), "true", true, false},
		{"bad bool", new(bool), "on", false, true},
		{"int8 min", new(int8), "-128", int8(-128), false},
		{"int8 overflow", new(int8), "128", int8(0), true},
		{"unsigned negative", new(uint16), "-1", uint16(0), true},
		{"float", new(float64), "1.25", float64(1.25), false},
		{"infinite", new(float64), "Inf", float64(0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := reflect.ValueOf(tc.target).Elem()
			err := BindScalarPath(value, "id", tc.raw)
			if (err != nil) != tc.invalid || !tc.invalid && value.Interface() != tc.want {
				t.Fatalf("value=%v err=%v", value, err)
			}
		})
	}
}
