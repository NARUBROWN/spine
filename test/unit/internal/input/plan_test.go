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
	if _, _, err := Inspect(reflect.TypeFor[markers.Path[int]]()); err == nil {
		t.Fatal("scalar payload accepted")
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
