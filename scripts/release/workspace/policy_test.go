package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPolicyRequiresEveryPlatformPinAndSource(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/release-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"nethttp-platform", "nats-platform"} {
		module := core + "-" + platform
		for _, channel := range append(append([]string{}, channels...), "ci_sources") {
			t.Run(platform+"/"+channel, func(t *testing.T) {
				doc, err := decodeDocument(raw)
				if err != nil {
					t.Fatal(err)
				}
				for name, suffix := range map[string]string{"preview": "-preview.1", "rc": "-rc.1", "stable": ""} {
					objectField(channelComponents(doc, name), "modules")[module] = "v0.1.0" + suffix
				}
				if _, err := decodePolicy(doc); err != nil {
					t.Fatal(err)
				}
				if channel == "ci_sources" {
					delete(objectField(doc, channel), module)
				} else {
					delete(objectField(channelComponents(doc, channel), "modules"), module)
				}
				if _, err := decodePolicy(doc); err == nil {
					t.Fatal("accepted a missing platform pin")
				}
			})
		}
	}
}

func TestPolicyRejectsInvalidCoreChannelVersion(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/release-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"core", "platform"} {
		doc, err := decodeDocument(raw)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "platform" {
			doc["kind"], doc["module"], doc["ci_core"] = kind, core+"-echo-platform", strings.Repeat("a", 40)
			delete(doc, "ci_sources")
			for _, channel := range channels {
				channelComponents(doc, channel)["modules"] = document{}
			}
		}
		if _, err := decodePolicy(doc); err != nil {
			t.Fatal(err)
		}
		for _, version := range []string{"v0.7.5", "v0.7.5-rc.1", "main", "v0.7.5-0.20261104000000-aaaaaaaaaaaa"} {
			channelComponents(doc, "preview")["core_version"] = version
			if _, err := decodePolicy(doc); err == nil {
				t.Errorf("%s accepted %s", kind, version)
			}
		}
	}
}

func TestObservationWindows(t *testing.T) {
	now := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		elapsed, required int
		override          string
		ok                bool
	}{
		{71, 72, "", false}, {72, 72, "", true},
		{23, 24, "", false}, {24, 24, "", true},
		{167, 168, "", false}, {168, 168, "", true},
		{71, 72, "emergency observation override", true},
		{-1, 72, "emergency observation override", false},
	} {
		at := now.Add(-time.Duration(tc.elapsed) * time.Hour).Format(time.RFC3339)
		if err := requireAge(at, tc.required, now, tc.override); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}

func TestPolicyAllowsPlatformTagsAcrossCoreChannels(t *testing.T) {
	raw, err := os.ReadFile("../../../.github/release-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := decodeDocument(raw)
	if err != nil {
		t.Fatal(err)
	}
	preview := objectField(channelComponents(doc, "preview"), "modules")
	for _, channel := range []string{"rc", "stable"} {
		pins := objectField(channelComponents(doc, channel), "modules")
		for module, version := range preview {
			pins[module] = version
		}
	}
	if _, err := decodePolicy(doc); err != nil {
		t.Fatal(err)
	}
	objectField(channelComponents(doc, "rc"), "modules")[core+"-nethttp-platform"] = "not-a-tag"
	if _, err := decodePolicy(doc); err == nil {
		t.Fatal("accepted invalid platform tag")
	}
}
