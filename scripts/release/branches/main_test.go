package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type mockAPI func(string, string, any, any, bool) (bool, error)

func (f mockAPI) API(method, path string, input, output any, missing bool) (bool, error) {
	return f(method, path, input, output, missing)
}

func respond(out, value any) (bool, error) {
	if out == nil {
		return true, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	err = json.Unmarshal(data, out)
	return err == nil, err
}

func TestBootstrapCreatesOnlyMissingChannels(t *testing.T) {
	for _, scenario := range []struct {
		apply, rcExists bool
		writes          []string
	}{
		{false, false, nil}, {true, false, []string{"refs/heads/rc"}}, {true, true, nil},
	} {
		source := strings.Repeat("a", 40)
		var writes []string
		repo := mockAPI(func(method, path string, input, out any, missing bool) (bool, error) {
			switch path {
			case "git/ref/heads/main":
				return respond(out, object{"object": object{"sha": source}})
			case "git/ref/heads/preview":
				return true, nil
			case "git/ref/heads/rc":
				return scenario.rcExists, nil
			case "git/refs":
				payload := input.(object)
				if method != "POST" || payload["sha"] != source {
					t.Fatal(method, payload)
				}
				writes = append(writes, payload["ref"].(string))
				return true, nil
			default:
				t.Fatal(path)
				return false, nil
			}
		})
		if err := bootstrap(repo, "NARUBROWN/spine", scenario.apply); err != nil || !reflect.DeepEqual(writes, scenario.writes) {
			t.Fatalf("writes=%v err=%v", writes, err)
		}
	}
}

func TestLegacyProtectionRequiresBackupBeforeDelete(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var sequence []string
		repo := mockAPI(func(method, path string, input, out any, missing bool) (bool, error) {
			sequence = append(sequence, method+" "+path)
			switch path {
			case "branches/main/protection":
				return respond(out, object{"required_pull_request_reviews": object{}})
			case "environments/release-stable":
				if method != "GET" || !missing {
					t.Fatal("existing stable approval must be preserved")
				}
			case "environments/release-preview/deployment-branch-policies":
				return respond(out, object{"branch_policies": []branchPolicy{{Name: "preview"}}})
			case "environments/release-rc/deployment-branch-policies":
				return respond(out, object{"branch_policies": []branchPolicy{{Name: "rc"}}})
			}
			return true, nil
		})
		err := applyEnvironments(repo, policy{}, func(string) (int64, error) {
			t.Fatal("reviewer lookup for existing environment")
			return 0, nil
		}, func(data json.RawMessage) error {
			if !json.Valid(data) || len(data) == 0 {
				t.Fatal("backup must receive protection data")
			}
			sequence = append(sequence, "backup")
			if fail {
				return errors.New("backup failed")
			}
			return nil
		})
		want := []string{"GET branches/main/protection", "backup"}
		if !fail {
			want = append(want, "DELETE branches/main/protection", "PATCH ", "PUT environments/release-preview", "GET environments/release-preview/deployment-branch-policies", "PUT environments/release-rc", "GET environments/release-rc/deployment-branch-policies", "GET environments/release-stable")
		}
		if (err != nil) != fail || !reflect.DeepEqual(sequence, want) {
			t.Fatalf("%v %v", sequence, err)
		}
	}
}
