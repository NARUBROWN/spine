package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func fixture() plan {
	return plan{Version: "v0.7.0-preview.2", SourceSHA: strings.Repeat("a", 40), Repository: "NARUBROWN/spine"}
}

func TestRoutesAndPayload(t *testing.T) {
	for _, tc := range []struct {
		suffix, channel, secret string
		color                   int
	}{
		{"-preview.2", "preview", "DISCORD_WEBHOOK_PREVIEW", 15105570},
		{"-rc.1", "rc", "DISCORD_WEBHOOK_RC", 3447003},
		{"", "stable", "DISCORD_WEBHOOK_RELEASE", 5763719},
	} {
		for _, project := range []string{"spine", "spine-cli", "spine-echo-platform", "spine-gorilla-platform", "spine-kafka-platform", "spine-rabbitmq-platform", "spine-nats-platform", "spine-validator-playground"} {
			p := fixture()
			p.Repository, p.Version, p.Channel = "NARUBROWN/"+project, "v0.7.0"+tc.suffix, &tc.channel
			channel, payload, err := buildPayload(p, "한글 \"이름\"\n", "https://github.com/runs/123", time.Unix(0, 0))
			if err != nil || channel != tc.channel || routes[channel].secret != tc.secret {
				t.Fatalf("channel=%q err=%v", channel, err)
			}
			e := payload["embeds"].([]map[string]any)[0]
			if e["color"] != tc.color || e["url"] != "https://github.com/"+p.Repository+"/tree/"+p.Version ||
				!strings.Contains(e["title"].(string), project) || !strings.Contains(e["description"].(string), "새 태그가 생성되었습니다") ||
				!strings.Contains(payload["avatar_url"].(string), "/NARUBROWN/spine/main/") ||
				!reflect.DeepEqual(payload["allowed_mentions"], map[string]any{"parse": []string{}}) {
				t.Fatal(payload)
			}
			if _, err := json.Marshal(payload); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestInvalidPlan(t *testing.T) {
	for _, mutate := range []func(*plan){
		func(p *plan) { p.Version = "v0.7.0-custom.1" },
		func(p *plan) { channel := "stable"; p.Channel = &channel },
		func(p *plan) { p.SourceSHA = "invalid" },
	} {
		p := fixture()
		mutate(&p)
		if _, _, err := buildPayload(p, "actor", "url", time.Time{}); err == nil {
			t.Fatalf("invalid plan accepted: %+v", p)
		}
	}
}

func TestTruncation(t *testing.T) {
	for _, s := range []string{strings.Repeat("한글 \"본문\"\n", 600), strings.Repeat("🚀", 300)} {
		got := shorten(s, 256)
		if len(utf16.Encode([]rune(got))) > 256 || !strings.HasSuffix(got, "...") || strings.ContainsRune(got, '\uFFFD') {
			t.Fatal(got)
		}
	}
}

func TestWebhookURL(t *testing.T) {
	got, err := webhookURL("https://discord.com/api/webhooks/123/token?thread_id=456&wait=false")
	if err != nil || got != "https://discord.com/api/webhooks/123/token?thread_id=456&wait=true" {
		t.Fatalf("%q %v", got, err)
	}
	for _, url := range []string{"http://discord.com/api/webhooks/123/token", "https://example.com/api/webhooks/123/token"} {
		if _, err := webhookURL(url); err == nil {
			t.Fatal(url)
		}
	}
}

func TestDelivery(t *testing.T) {
	for _, tc := range []struct {
		code          int
		body, problem string
	}{
		{200, `{"id":"789"}`, ""}, {400, `{}`, "HTTP 400"}, {429, `{}`, "HTTP 429"}, {500, `{}`, "HTTP 500"}, {200, `{}`, "메시지"},
	} {
		t.Run(fmt.Sprintf("%d/%s", tc.code, tc.problem), func(t *testing.T) {
			body := []byte(`{"username":"Spine Bot"}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var got map[string]string
				if r.Method != "POST" || r.URL.Query().Get("wait") != "true" || r.Header.Get("Content-Type") != "application/json" ||
					json.NewDecoder(r.Body).Decode(&got) != nil || got["username"] != "Spine Bot" {
					t.Error("unexpected webhook request")
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			err := send(server.Client(), server.URL+"/api/webhooks/123/test-token?wait=true", body)
			if tc.problem == "" && err != nil || tc.problem != "" && (err == nil || !strings.Contains(err.Error(), tc.problem)) {
				t.Fatalf("send: %v", err)
			}
		})
	}
}

func TestCallerWorkspaceDryRunAndMissingSecret(t *testing.T) {
	t.Chdir(t.TempDir())
	p := fixture()
	p.Repository = "NARUBROWN/spine-cli"
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("release-plan.json", data, 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"NOTIFICATION_PLAN": "release-plan.json", "GITHUB_REPOSITORY": p.Repository, "GITHUB_ACTOR": "NARUBROWN", "GITHUB_RUN_ID": "123"}
	getenv := func(key string) string { return env[key] }
	var output bytes.Buffer
	if err := run([]string{"--dry-run"}, getenv, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Embeds []struct {
			Color int
			Title string
		}
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Embeds) != 1 || result.Embeds[0].Color != 15105570 || !strings.Contains(result.Embeds[0].Title, filepath.Base(p.Repository)) {
		t.Fatal(output.String())
	}
	if err := run(nil, getenv, &output); err == nil || !strings.Contains(err.Error(), "DISCORD_WEBHOOK_PREVIEW") {
		t.Fatalf("missing secret: %v", err)
	}
}
