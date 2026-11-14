package sourceproof

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type fixture map[string]any

func (f fixture) API(method, path string, input, output any, missing bool) (bool, error) {
	value, exists := f[path]
	if method != "GET" || !exists {
		return false, fmt.Errorf("unexpected request %s %s", method, path)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	err = json.Unmarshal(payload, output)
	return err == nil, err
}

func TestFingerprintMatchesExistingManifestAlgorithm(t *testing.T) {
	entry := func(path, sha string) map[string]string {
		return map[string]string{"path": path, "sha": sha, "mode": "100644", "type": "blob"}
	}
	blob := func(data string) map[string]string {
		return map[string]string{"content": base64.StdEncoding.EncodeToString([]byte(data))}
	}
	repo := fixture{
		"git/commits/source":         map[string]any{"tree": map[string]string{"sha": "tree"}},
		"git/trees/tree?recursive=1": map[string]any{"tree": []map[string]string{entry("한글🚀.go", "foo"), entry("go.mod", "mod"), entry("test/go.sum", "sum")}},
		"git/blobs/mod":              blob("module sample\n\nrequire (\n\tgithub.com/NARUBROWN/spine v0.7.5-rc.1 // indirect\n\texample.org/mod v1.2.3\n)\n"),
		"git/blobs/sum":              blob("github.com/NARUBROWN/spine v0.7.5-rc.1 h1:old\nexample.org/mod v1.2.3 h1:keep\n"),
	}
	const want = "d016f31e1eba494733f24bb8cc040bc94eed7a5b17fc31944618799559c76430"
	if got, err := Fingerprint(repo, "source"); err != nil || got != want {
		t.Fatalf("%s, %v", got, err)
	}
	repo["git/trees/tree?recursive=1"] = map[string]any{"truncated": true, "tree": []any{}}
	if _, err := Fingerprint(repo, "source"); err == nil {
		t.Fatal("truncated tree accepted")
	}
}

func TestNormalizeOnlyManagedChannelMetadata(t *testing.T) {
	before := "module example.com/app\nrequire (\n github.com/NARUBROWN/spine v0.7.0-preview.1\n example.com/native v1.2.3\n)\n"
	after := strings.ReplaceAll(before, "v0.7.0-preview.1", "v0.7.0-rc.2")
	for _, tc := range []struct {
		path, left, right string
		equal             bool
	}{
		{"go.mod", before, after, true},
		{"go.mod", before, strings.ReplaceAll(after, "v0.7.0-rc.2", "v0.8.0-rc.1"), false},
		{"go.mod", before, strings.ReplaceAll(after, "v1.2.3", "v1.2.4"), false},
		{"go.mod", before, after + "replace example.com/native => ../local\n", false},
		{"main.go", "package first", "package second", false},
		{"go.mod", "require " + core + " v0.7.0-preview.1\n", "require " + core + " v0.7.0-rc.1\n", true},
		{"go.mod", "exclude (\n " + core + " v0.7.0-preview.1\n)\n", "exclude (\n " + core + " v0.7.0-rc.1\n)\n", false},
		{".github/release-policy.json", `{"channels":{},"ci_sources":{"a":"1"},"preview_hours":72}`, `{"channels":{},"ci_sources":{"a":"2"},"preview_hours":72}`, true},
		{".github/release-policy.json", `{"channels":{},"preview_hours":72}`, `{"channels":{},"preview_hours":73}`, false},
	} {
		a, err := normalize(tc.path, []byte(tc.left))
		if err != nil {
			t.Fatal(err)
		}
		b, err := normalize(tc.path, []byte(tc.right))
		if err != nil {
			t.Fatal(err)
		}
		if (string(a) == string(b)) != tc.equal {
			t.Errorf("%s: %q versus %q", tc.path, a, b)
		}
	}
}
