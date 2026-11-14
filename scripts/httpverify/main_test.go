package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestVerifyRequiresPassingTests(t *testing.T) {
	for _, mode := range []string{"v3", "platforms"} {
		for _, passing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/pass=%t", mode, passing), func(t *testing.T) {
				core := t.TempDir()
				echo, nethttp := filepath.Join(core, "echo"), filepath.Join(core, "nethttp")
				for _, root := range []string{core, echo, nethttp, filepath.Join(core, "test"), filepath.Join(core, "test", "openapi"), filepath.Join(core, "test", "http-platform")} {
					if err := os.MkdirAll(root, 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				output := filepath.Join(core, "output")
				overlay := make(map[string]bool)
				runner := func(_ string, _ []string, w io.Writer, args ...string) (int, error) {
					if len(args) > 3 && args[0] == "run" && args[1] == "./scripts/test" {
						overlay[args[2]] = true
					}
					if passing && slices.Contains(args, "-json") {
						_, err := fmt.Fprintln(w, `{"Action":"pass","Package":"fixture","Test":"TestOK"}`)
						return 0, err
					}
					return 0, nil
				}
				ok, err := verify(mode, core, echo, nethttp, output, runner, io.Discard)
				if err != nil || ok != passing {
					t.Fatalf("verify = %t, %v", ok, err)
				}
				data, err := os.ReadFile(filepath.Join(output, "summary.json"))
				if err != nil {
					t.Fatal(err)
				}
				var report map[string]any
				names := []string{"core", "echo", "integration", "openapi"}
				if mode == "platforms" {
					names = []string{"nethttp", "platform-e2e"}
				}
				want := map[string]any{"success": passing, "manifests_unchanged": true}
				for _, name := range names {
					passes := float64(0)
					if passing {
						passes = 1
					}
					want[name] = map[string]any{"pass": passes, "fail": float64(0), "skip": float64(0),
						"exit_code": float64(0), "vet_exit_code": float64(0)}
				}
				if err := json.Unmarshal(data, &report); err != nil || !reflect.DeepEqual(report, want) {
					t.Fatalf("summary = %s, want %v, error %v", data, want, err)
				}
				if mode == "v3" && (!overlay["test"] || !overlay["vet"]) {
					t.Fatal("core test overlay was bypassed")
				}
			})
		}
	}
}

func TestSummaryKeepsOnlyLeavesWithinEachPackage(t *testing.T) {
	events := strings.Join([]string{
		`not JSON`,
		`{"Action":"pass","Package":"a","Test":"TestParent"}`,
		`{"Action":"skip","Package":"a","Test":"TestParent/child"}`,
		`{"Action":"pass","Package":"b","Test":"TestParent"}`,
	}, "\n")
	result, err := summarize(bytes.NewBufferString(events))
	if err != nil {
		t.Fatal(err)
	}
	if result["pass"] != 1 || result["skip"] != 1 || result["fail"] != 0 {
		t.Fatal(result)
	}
}
