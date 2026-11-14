package publiccheck

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPublicTestsRequireRunAndPassWithoutFailures(t *testing.T) {
	start := "{\"Action\":\"run\",\"Test\":\"TestOK\"}\n"
	pass := "{\"Action\":\"pass\",\"Test\":\"TestOK\"}\n"
	for _, test := range []struct {
		events string
		err    error
		want   bool
	}{
		{"", nil, false}, {start, nil, false}, {pass, nil, false},
		{`{"Action":"skip","Package":"helper"}`, nil, false},
		{`{"Action":"skip","Test":"TestBroker"}`, nil, false},
		{start + `{"Action":"skip","Test":"TestOK"}`, nil, false},
		{start + pass, nil, true}, {start + pass, errors.New("exit 1"), false},
		{start + pass + `{"Action":"skip","Package":"helper"}`, nil, true},
		{start + pass + `{"Action":"skip","Test":"TestBroker"}`, nil, false},
		{start + pass + `{"Action":"fail","Package":"broken"}`, nil, false},
	} {
		r := Runner{Output: io.Discard, command: func(string, time.Duration, []string) ([]byte, []byte, error) {
			return []byte(test.events), nil, test.err
		}}
		if err := r.Tests(".", "./..."); (err == nil) != test.want {
			t.Fatalf("%q: %v", test.events, err)
		}
	}
}

func TestPublicChecksumRetriesOnlyMatching404(t *testing.T) {
	target := Core + "@v0.7.5"
	missing := target + ": verifying module: reading https://sum.golang.org/lookup/github.com/!n!a!r!u!b!r!o!w!n/spine@v0.7.5: 404 Not Found"
	for _, scenario := range []struct {
		message   string
		successAt int
		delays    []time.Duration
		pass      bool
	}{
		{missing, 2, []time.Duration{15 * time.Second}, true},
		{missing, 0, []time.Duration{15 * time.Second, 30 * time.Second, 60 * time.Second, 120 * time.Second}, false},
		{strings.ReplaceAll(missing, "404 Not Found", "500 Error"), 0, nil, false},
		{strings.ReplaceAll(missing, target, "other"), 0, nil, false},
		{"404 Not Found", 0, nil, false},
		{"checksum mismatch", 0, nil, false},
	} {
		var sleeps []time.Duration
		calls := 0
		r := Runner{Output: io.Discard, sleep: func(d time.Duration) { sleeps = append(sleeps, d) }, command: func(_ string, _ time.Duration, args []string) ([]byte, []byte, error) {
			calls++
			if !reflect.DeepEqual(args, []string{"go", "mod", "download", "-json", target}) {
				t.Fatal(args)
			}
			if calls == scenario.successAt {
				return nil, nil, nil
			}
			return []byte(scenario.message), nil, errors.New("failed download")
		}}
		if err := r.waitForTag(Core, "v0.7.5", "."); (err == nil) != scenario.pass || calls != len(scenario.delays)+1 || !reflect.DeepEqual(sleeps, scenario.delays) {
			t.Fatalf("%v, %v", err, sleeps)
		}
	}
}

func TestCoreReportNeedsNoBrokerAndKeepsOriginalComponents(t *testing.T) {
	const version = "v0.7.5"
	plan := Plan{Module: Core, Kind: "core", Version: version, SourceSHA: strings.Repeat("a", 40), Components: json.RawMessage(`{"core_version":"v0.7.5","modules":{"other":"v1.0.0"}}`)}
	work := t.TempDir()
	r := Runner{Output: io.Discard, command: func(_ string, _ time.Duration, args []string) ([]byte, []byte, error) {
		switch strings.Join(args, " ") {
		case "go list -m -json " + Core + "@" + version, "go list -m -json all":
			return []byte(`{"Path":"` + Core + `","Version":"` + version + `"}`), nil, nil
		case "go mod download -json " + Core + "@" + version,
			"go mod edit -require=" + Core + "@" + version, "go mod tidy", "go mod verify",
			"go build -mod=readonly -o " + filepath.Join(work, "consumer-app") + " .":
			return nil, nil, nil
		case "go version":
			return []byte("go version go1.26.0 darwin/arm64\n"), nil, nil
		}
		t.Fatalf("unexpected core-only command: %v", args)
		return nil, nil, nil
	}}
	report, err := r.Verify(plan, ".", work, version, "core")
	if err != nil || report.Result != "passed" || report.Version != version || report.SourceSHA != plan.SourceSHA || report.Scope != "core" || !reflect.DeepEqual(report.Components, plan.Components) || !reflect.DeepEqual(report.Selected, map[string]string{Core: version}) {
		t.Fatalf("%+v %v", report, err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var got, want map[string]any
	const expected = `{
		"result": "passed",
		"scope": "core",
		"source_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"version": "v0.7.5",
		"components": {"core_version": "v0.7.5", "modules": {"other": "v1.0.0"}},
		"selected_modules": {"github.com/NARUBROWN/spine": "v0.7.5"},
		"go_version": "go version go1.26.0 darwin/arm64",
		"evidence": ["published modules without go.work or replace", "consumer build", "scope=core"]
	}`
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(expected), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("report = %s, want %s", encoded, expected)
	}

	if _, err := r.Verify(plan, ".", t.TempDir(), version, "cohort"); err == nil || !strings.Contains(err.Error(), "SPINE_TEST_KAFKA_BROKER") {
		t.Fatalf("%v", err)
	}
	r.Env = map[string]string{"SPINE_TEST_KAFKA_BROKER": "broker", "SPINE_TEST_RABBITMQ_URL": "rabbit"}
	if _, err := r.Verify(plan, ".", t.TempDir(), version, "cohort"); err == nil || !strings.Contains(err.Error(), "SPINE_TEST_NATS_URL") {
		t.Fatalf("%v", err)
	}
	if _, err := r.Verify(plan, ".", t.TempDir(), version, "platform"); err == nil || !strings.Contains(err.Error(), "only to platforms") {
		t.Fatalf("%v", err)
	}
}

func TestModulesRejectUnexpectedReplacementsAndWrongVersions(t *testing.T) {
	for _, tc := range []struct {
		extra    string
		expected string
		ok       bool
	}{
		{"", "v0.7.5", true},
		{"", "v0.7.4", false},
		{`{"Path":"example.com/other","Replace":{"Path":"../other"}}`, "v0.7.5", false},
		{`{"Path":"github.com/NARUBROWN/spine-simple-msa","Replace":{"Path":"../shared"}}`, "v0.7.5", true},
	} {
		r := Runner{command: func(string, time.Duration, []string) ([]byte, []byte, error) {
			return []byte(`{"Path":"` + Core + `","Version":"v0.7.5"}` + "\n" + tc.extra), nil, nil
		}}
		if _, err := r.Modules(".", map[string]string{Core: tc.expected}); (err == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestPlatformAndCohortPublicVerification(t *testing.T) {
	const version = "v0.7.5"
	const peer = Core + "-echo-platform"
	const peerVersion = "v0.2.0"
	const testCommand = "go test -json -mod=readonly -race ./... -count=1 -timeout=4m"
	for _, scope := range []string{"platform", "cohort"} {
		t.Run(scope, func(t *testing.T) {
			source, work := t.TempDir(), t.TempDir()
			suites := []string{"platform-e2e", "nats-e2e", "openapi", "http-platform"}
			for _, name := range suites {
				if err := os.MkdirAll(filepath.Join(source, "test", name), 0755); err != nil {
					t.Fatal(err)
				}
			}
			module, query := Core, version
			want := suites
			kind := "core"
			if scope == "platform" {
				module, query, kind = peer, peerVersion, "platform"
				want = []string{"published-platform", "vet:published-platform"}
			}
			components := json.RawMessage(`{"core_version":"` + version + `","modules":{"` + peer + `":"` + peerVersion + `"}}`)
			var calls []string
			r := Runner{Output: io.Discard, Env: map[string]string{
				"SPINE_TEST_KAFKA_BROKER": "kafka", "SPINE_TEST_RABBITMQ_URL": "rabbit", "SPINE_TEST_NATS_URL": "nats",
			}, command: func(dir string, _ time.Duration, args []string) ([]byte, []byte, error) {
				switch command := strings.Join(args, " "); command {
				case "go list -m -json " + module + "@" + query:
					return []byte(`{"Version":"` + query + `"}`), nil, nil
				case "go mod download -json " + module + "@" + query:
					if dir == filepath.Join(work, "consumer") {
						data, err := json.Marshal(map[string]string{"Dir": filepath.Join(source, "test", suites[0])})
						return data, nil, err
					}
				case "go list -m -json all":
					return []byte(`{"Path":"` + Core + `","Version":"` + version + `"} {"Path":"` + peer + `","Version":"` + peerVersion + `"}`), nil, nil
				case "go mod edit -json":
					return []byte(`{"Require":[{"Path":"` + peer + `"}]}`), nil, nil
				case testCommand:
					calls = append(calls, filepath.Base(dir))
					return []byte("{\"Action\":\"run\",\"Test\":\"TestOK\"}\n{\"Action\":\"pass\",\"Test\":\"TestOK\"}\n"), nil, nil
				case "go vet -mod=readonly ./...":
					calls = append(calls, "vet:"+filepath.Base(dir))
				case "go version":
					return []byte("go version fixture\n"), nil, nil
				default:
					if !strings.HasPrefix(command, "go mod edit -require=") && command != "go mod tidy" &&
						command != "go mod verify" && !strings.HasPrefix(command, "go build -mod=readonly -o ") {
						t.Fatalf("unexpected command %s", command)
					}
				}
				return nil, nil, nil
			}}
			got, err := r.Verify(Plan{Kind: kind, Module: module, Version: query, SourceSHA: "source", Components: components}, source, work, query, scope)
			expected := Report{
				Result: "passed", Scope: scope, SourceSHA: "source", Version: query, Components: components,
				Selected: map[string]string{Core: version, peer: peerVersion}, GoVersion: "go version fixture",
				Evidence: []string{"published modules without go.work or replace", "consumer build", "scope=" + scope},
			}
			if err != nil || !reflect.DeepEqual(got, expected) || !reflect.DeepEqual(calls, want) {
				t.Fatalf("report %+v, suites %v, error %v", got, calls, err)
			}
		})
	}
}
