package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSwaggerPatchReproducesBundle(t *testing.T) {
	u := updater{root: "../.."}
	var patches swaggerPatches
	if err := u.readJSON(toolsPath+"/swagger-ui-fixes.json", &patches); err != nil {
		t.Fatal(err)
	}
	var manifest manifest
	if err := u.readJSON(toolsPath+"/manifest.json", &manifest); err != nil {
		t.Fatal(err)
	}
	version := manifest.Distributions["swagger-ui-dist"].Version
	shipped, err := os.ReadFile("../../pkg/openapi/ui/swaggerui/assets/swagger-ui-bundle.js")
	if err != nil {
		t.Fatal(err)
	}
	original, found := bytes.CutPrefix(shipped, []byte(patches.Notice))
	if !found {
		t.Fatal("missing patch notice")
	}
	for i := len(patches.Replacements) - 1; i >= 0; i-- {
		patch := patches.Replacements[i]
		if bytes.Count(original, []byte(patch.After)) != 1 {
			t.Fatalf("patch %s does not occur exactly once", patch.Issue)
		}
		original = bytes.Replace(original, []byte(patch.After), []byte(patch.Before), 1)
	}
	if digest(original) != patches.UpstreamSHA256 {
		t.Fatal("upstream digest differs")
	}
	got, err := patchSwagger(original, version, patches)
	if err != nil || !bytes.Equal(got, shipped) {
		t.Fatalf("bundle differs: %v", err)
	}
	if _, err := patchSwagger(original, "5.34.2", patches); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unexpected version error: %v", err)
	}
	if _, err := patchSwagger(shipped, version, patches); err == nil || !strings.Contains(err.Error(), "reviewed upstream bundle") {
		t.Fatalf("unexpected hash error: %v", err)
	}
}

func TestLicenseExpressions(t *testing.T) {
	for _, expression := range []string{"MIT", "(MPL-2.0 OR Apache-2.0)", "(MIT OR GPL-3.0) AND BSD-3-Clause"} {
		if !permitted(expression) {
			t.Errorf("valid expression rejected: %s", expression)
		}
	}
	for _, expression := range []string{"GPL-3.0", "MIT AND GPL-3.0", "(MIT OR BSD-3-Clause) AND GPL-3.0", "MIT WITH unknown-exception", "MIT OR", "(MIT"} {
		if permitted(expression) {
			t.Errorf("invalid expression accepted: %s", expression)
		}
	}
}

func TestArchiveIntegrityAndTypingsRoot(t *testing.T) {
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	payload := []byte("MIT license terms")
	if err := tw.WriteHeader(&tar.Header{Name: "hast/LICENSE", Size: int64(len(payload)), Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha512.Sum512(data.Bytes())
	entry := lockEntry{Resolved: registry + "fixture.tgz", Integrity: "sha512-" + base64.StdEncoding.EncodeToString(sum[:])}
	u := updater{cache: t.TempDir(), download: func(string) ([]byte, error) { return []byte("changed"), nil }}
	if _, err := u.archive(entry); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("unexpected integrity error: %v", err)
	}
	u.download = func(string) ([]byte, error) { return data.Bytes(), nil }
	got, err := u.archive(entry)
	if err != nil || !bytes.Equal(got["LICENSE"], payload) {
		t.Fatalf("archive: %q %v", got, err)
	}
}

func TestCheckRejectsFileHashDriftOffline(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, toolsPath), 0755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		toolsPath + "/package.json":  `{"dependencies":{"redoc":"2.5.4"}}`,
		toolsPath + "/manifest.json": `{"distributions":{"redoc":{"version":"2.5.4"}},"files":{"asset.js":"` + digest([]byte("expected")) + `"}}`,
		"asset.js":                   "changed",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	u := updater{root: root, download: func(string) ([]byte, error) { t.Fatal("offline check tried to download"); return nil, nil }}
	if err := u.check(); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("unexpected verification error: %v", err)
	}
}
