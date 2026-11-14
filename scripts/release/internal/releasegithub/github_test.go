package releasegithub

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestTagLookupDistinguishesMissingAndAuthentication(t *testing.T) {
	c, err := New("NARUBROWN/spine", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []int{404, 401, 403, 500} {
		for _, value := range []string{fmt.Sprint(status), fmt.Sprintf("%q", fmt.Sprint(status))} {
			c.command = func([]string, []byte) ([]byte, string, error) {
				return []byte(`{"status":` + value + `}`), "failure", errors.New("exit 1")
			}
			got, err := c.TagSHA("v0.7.0-preview.1")
			if got != "" || (err == nil) != (status == 404) {
				t.Fatalf("status %s: %q %v", value, got, err)
			}
		}
	}
	c.command = func(args []string, _ []byte) ([]byte, string, error) {
		if strings.Contains(args[3], "git/ref/tags/") {
			return []byte(`{"object":{"type":"tag","sha":"annotated"}}`), "", nil
		}
		return []byte(`{"object":{"type":"commit","sha":"resolved"}}`), "", nil
	}
	if got, err := c.TagSHA("v0.7.0-preview.1"); err != nil || got != "resolved" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestCandidateEvidence(t *testing.T) {
	const version = "v0.7.0-preview.1"
	manifest := `{"version":"` + version + `","source_sha":"` + strings.Repeat("a", 40) + `"}`
	var data bytes.Buffer
	zipped := zip.NewWriter(&data)
	file, err := zipped.Create("release-manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := zipped.Close(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "missing", "expired", "duplicate", "bad hash", "missing allowed", "expired allowed", "later page", "corrupt ZIP", "missing manifest", "download failure"} {
		t.Run(scenario, func(t *testing.T) {
			c, err := New("NARUBROWN/spine", "")
			if err != nil {
				t.Fatal(err)
			}
			archive := data.Bytes()
			if scenario == "corrupt ZIP" {
				archive = []byte("corrupt")
			}
			if scenario == "missing manifest" {
				var buffer bytes.Buffer
				writer := zip.NewWriter(&buffer)
				if _, err := writer.Create("wrong.json"); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				archive = buffer.Bytes()
			}
			digest := fmt.Sprintf("sha256:%x", sha256.Sum256(archive))
			if scenario == "bad hash" {
				digest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("different archive")))
			}
			item := fmt.Sprintf(`{"id":1,"name":"release-manifest-%s","expired":%t,"digest":%q,"workflow_run":{"id":10}}`, version, strings.HasPrefix(scenario, "expired"), digest)
			rows := item
			switch scenario {
			case "missing", "missing allowed":
				rows = ""
			case "duplicate":
				rows = item + "," + item
			}
			c.command = func(args []string, _ []byte) ([]byte, string, error) {
				endpoint := args[len(args)-1]
				switch {
				case strings.Contains(endpoint, "actions/artifacts?"):
					if scenario == "later page" && strings.HasSuffix(endpoint, "&page=1") {
						unrelated := make([]map[string]string, 100)
						for i := range unrelated {
							unrelated[i] = map[string]string{"name": "unrelated"}
						}
						raw, err := json.Marshal(map[string]any{"artifacts": unrelated})
						return raw, "", err
					}
					return []byte(`{"artifacts":[` + rows + `]}`), "", nil
				case strings.HasSuffix(endpoint, "actions/runs/10"):
					return []byte(`{"id":10,"conclusion":"success"}`), "", nil
				case endpoint == "repos/NARUBROWN/spine/actions/artifacts/1/zip":
					if !slices.Equal(args, []string{"api", endpoint}) {
						t.Fatal(args)
					}
					switch scenario {
					case "missing", "expired", "duplicate", "missing allowed", "expired allowed":
						t.Fatal("unexpected artifact download")
					case "download failure":
						return nil, "artifact unavailable", errors.New("exit 1")
					}
					return archive, "", nil
				}
				t.Fatalf("unexpected request %v", args)
				return nil, "", nil
			}
			allowed := strings.HasSuffix(scenario, "allowed")
			proof, err := c.CandidateEvidence(version, "release-manifest-", allowed)
			switch {
			case scenario == "valid" || scenario == "later page":
				if err != nil || proof == nil || proof.Artifact.WorkflowRun.ID != 10 || string(proof.Manifest) != manifest || string(proof.WorkflowRun) != `{"id":10,"conclusion":"success"}` {
					t.Fatalf("%+v %v", proof, err)
				}
			case allowed:
				if err != nil || proof != nil {
					t.Fatalf("%+v %v", proof, err)
				}
			default:
				if err == nil {
					t.Fatal("invalid evidence accepted")
				}
				if scenario == "download failure" && !strings.Contains(err.Error(), "download") {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestRepositorySettingsEndpointHasNoTrailingSlash(t *testing.T) {
	c, err := New("NARUBROWN/spine", "")
	if err != nil {
		t.Fatal(err)
	}
	c.command = func(args []string, input []byte) ([]byte, string, error) {
		want := []string{"api", "--method", "PATCH", "repos/NARUBROWN/spine", "--input", "-"}
		if !slices.Equal(args, want) || string(input) != `{"default_branch":"preview"}` {
			t.Fatalf("arguments %v, input %s", args, input)
		}
		return input, "", nil
	}
	var got map[string]string
	ok, err := c.API("PATCH", "", map[string]string{"default_branch": "preview"}, &got, false)
	if err != nil || !ok || got["default_branch"] != "preview" {
		t.Fatalf("response %v, found %v, error %v", got, ok, err)
	}
}
