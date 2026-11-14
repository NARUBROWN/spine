package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/scripts/release/internal/publiccheck"
)

type exampleFixture struct {
	pins       []publiccheck.GoModule
	operations *[]string
}

func (f exampleFixture) ReadModule(dir string) (publiccheck.ModuleEdit, error) {
	*f.operations = append(*f.operations, "read "+filepath.Base(dir))
	return publiccheck.ModuleEdit{Require: f.pins}, nil
}
func (f exampleFixture) Execute(dir string, args ...string) error {
	*f.operations = append(*f.operations, filepath.Base(dir)+" "+strings.Join(args, " "))
	return nil
}
func (f exampleFixture) Modules(dir string, pins map[string]string) (map[string]string, error) {
	if pins[core] != "v0.7.5" {
		return nil, fmt.Errorf("unexpected core pin %v", pins)
	}
	*f.operations = append(*f.operations, "graph "+filepath.Base(dir))
	return pins, nil
}
func (f exampleFixture) Tests(dir string, args ...string) error {
	return f.Execute(dir, append([]string{"test"}, args...)...)
}

func TestPostReleaseExamples(t *testing.T) {
	const version = "v0.7.5"
	specs := document{}
	for _, name := range []string{"chat", "msa"} {
		specs[name] = document{"repository": "NARUBROWN/example-" + name, "ref": strings.Repeat("a", 40)}
	}
	raw := document{"post_release_examples": specs}
	components := document{"core_version": version, "modules": document{}}
	for _, scenario := range []string{"valid", "preview", "rc", "wrong branch", "missing broker", "old pin"} {
		t.Run(scenario, func(t *testing.T) {
			var operations []string
			f := exampleFixture{pins: []publiccheck.GoModule{{Path: core, Version: version}}, operations: &operations}
			ref, query := "refs/heads/main", version
			env := map[string]string{"SPINE_TEST_KAFKA_BROKER": "broker", "SPINE_TEST_RABBITMQ_URL": "rabbit"}
			switch scenario {
			case "preview":
				query += "-preview.1"
			case "rc":
				query += "-rc.1"
			case "wrong branch":
				ref = "refs/heads/preview"
			case "missing broker":
				delete(env, "SPINE_TEST_RABBITMQ_URL")
			case "old pin":
				f.pins[0].Version = "v0.6.5"
			}
			report, err := verifyExamples(query, ref, t.TempDir(), raw, env, f, f, func() (document, error) {
				operations = append(operations, "proof")
				return document{"components": components}, nil
			}, func(dir, repo, sha string) error {
				operations = append(operations, "checkout "+filepath.Base(dir))
				return nil
			})
			if scenario != "valid" {
				var want []string
				if scenario == "old pin" {
					want = []string{"proof", "checkout chat", "checkout msa", "read chat"}
				}
				if err == nil || report != nil || !reflect.DeepEqual(operations, want) {
					t.Fatalf("%v %v %v", report, err, operations)
				}
				return
			}
			want := []string{"proof", "checkout chat", "checkout msa"}
			for _, name := range []string{"chat", "order-app", "stock-app"} {
				want = append(want, "read "+name, name+" go mod download", name+" go mod verify", "graph "+name, name+" test -race ./... -count=1")
			}
			want = append(want, "chat test -race -tags=e2e ./e2e -count=1 -timeout=3m", "msa test -race -tags=e2e ./e2e -count=1 -timeout=4m")
			if err != nil || !reflect.DeepEqual(operations, want) || !reflect.DeepEqual(report, document{"result": "passed", "version": version, "examples": specs, "components": components}) {
				t.Fatalf("%v %v %v", report, err, operations)
			}
		})
	}
}
