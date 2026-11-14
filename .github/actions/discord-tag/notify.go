package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type route struct {
	secret string
	color  int
	title  string
}

var routes = map[string]route{
	"preview": {"DISCORD_WEBHOOK_PREVIEW", 15105570, "🧪 Preview Tag"},
	"rc":      {"DISCORD_WEBHOOK_RC", 3447003, "📦 Release Candidate Tag"},
	"stable":  {"DISCORD_WEBHOOK_RELEASE", 5763719, "🚀 Stable Tag"},
}
var tagPattern = regexp.MustCompile(`^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-(preview|rc)\.[1-9][0-9]*)?$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var errNotification = errors.New("Discord 태그 알림을 보내지 못했습니다. 채널별 secret과 webhook 응답을 확인하세요.")

type plan struct {
	Version    string  `json:"version"`
	Channel    *string `json:"channel"`
	SourceSHA  string  `json:"source_sha"`
	Repository string  `json:"repository"`
}

func units(r rune) int {
	if r > 0xffff {
		return 2
	}
	return 1
}

func shorten(value string, limit int) string {
	size := 0
	for _, r := range value {
		size += units(r)
	}
	if size <= limit {
		return value
	}
	size = 0
	for i, r := range value {
		size += units(r)
		if size > limit-3 {
			return value[:i] + "..."
		}
	}
	return value
}

func buildPayload(p plan, actor, runURL string, now time.Time) (string, map[string]any, error) {
	match := tagPattern.FindStringSubmatch(p.Version)
	if match == nil {
		return "", nil, errors.New("태그는 vX.Y.Z-preview.N, vX.Y.Z-rc.N 또는 vX.Y.Z 형식이어야 합니다")
	}
	channel := match[1]
	if channel == "" {
		channel = "stable"
	}
	if p.Channel != nil && channel != *p.Channel {
		return "", nil, errors.New("태그와 발행 계획의 채널이 일치하지 않습니다")
	}
	if !shaPattern.MatchString(p.SourceSHA) {
		return "", nil, errors.New("태그의 소스 SHA가 올바르지 않습니다")
	}
	selected := routes[channel]
	project := p.Repository[strings.LastIndexByte(p.Repository, '/')+1:]
	return channel, map[string]any{
		"username":         "Spine Bot",
		"avatar_url":       "https://raw.githubusercontent.com/NARUBROWN/spine/main/assets/spine-logo.png",
		"allowed_mentions": map[string]any{"parse": []string{}},
		"embeds": []map[string]any{{
			"title":       shorten(selected.title+" - "+project+" "+p.Version, 256),
			"url":         "https://github.com/" + p.Repository + "/tree/" + url.PathEscape(p.Version),
			"color":       selected.color,
			"description": "새 태그가 생성되었습니다. 릴리스 검증과 배포 상태는 프로젝트의 발행 기록에서 확인하세요.",
			"fields": []map[string]any{
				{"name": "🏷️ Tag", "value": "`" + shorten(p.Version, 1022) + "`", "inline": true},
				{"name": "👤 실행자", "value": "[" + actor + "](https://github.com/" + actor + ")", "inline": true},
				{"name": "실행 기록", "value": "[GitHub Actions에서 확인](" + runURL + ")"},
			},
			"footer":    map[string]string{"text": "Spine Web Framework | GitHub Actions"},
			"timestamp": now.UTC().Format("2006-01-02T15:04:05+00:00"),
		}},
	}, nil
}

func webhookURL(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Scheme != "https" || (strings.ToLower(u.Hostname()) != "discord.com" && strings.ToLower(u.Hostname()) != "discordapp.com") || u.User != nil || (u.Port() != "" && u.Port() != "443") || !strings.HasPrefix(u.EscapedPath(), "/api/webhooks/") {
		return "", errors.New("Discord webhook URL 형식을 확인하세요")
	}
	q := make(url.Values)
	for key, values := range u.Query() {
		for _, value := range values {
			if value != "" {
				q.Set(key, value)
			}
		}
	}
	q.Set("wait", "true")
	u.RawQuery, u.Fragment, u.RawFragment = q.Encode(), "", ""
	return u.String(), nil
}

func send(client *http.Client, target string, payload []byte) error {
	connectionError := errors.New("Discord 전송 실패: 연결 또는 응답 형식을 확인하세요")
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return connectionError
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Spine-Release-Notifier")
	response, err := client.Do(req)
	if err != nil {
		return connectionError
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Discord 전송 실패: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return connectionError
	}
	var message any
	if err := json.Unmarshal(body, &message); err != nil {
		return connectionError
	}
	if object, ok := message.(map[string]any); ok {
		if id, ok := object["id"].(string); ok && id != "" {
			return nil
		}
	}
	return errors.New("Discord에서 메시지 생성 확인을 받지 못했습니다")
}

func run(args []string, getenv func(string) string, output io.Writer) error {
	filename := getenv("NOTIFICATION_PLAN")
	if filename == "" {
		filename = "release-plan.json"
	}
	flags := flag.NewFlagSet("discord-tag", flag.ContinueOnError)
	path := flags.String("plan", filename, "release plan JSON")
	dryRun := flags.Bool("dry-run", false, "print the payload")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errNotification
	}
	data, err := os.ReadFile(*path)
	if err != nil {
		return errNotification
	}
	var p plan
	if err := json.Unmarshal(data, &p); err != nil {
		return errNotification
	}
	repository, actor, runID := getenv("GITHUB_REPOSITORY"), getenv("GITHUB_ACTOR"), getenv("GITHUB_RUN_ID")
	if repository == "" || actor == "" || runID == "" || p.Repository == "" {
		return errNotification
	}
	if p.Repository != repository {
		return errors.New("발행 계획과 실행 저장소가 일치하지 않습니다")
	}
	channel, payload, err := buildPayload(p, actor, "https://github.com/"+repository+"/actions/runs/"+runID, time.Now())
	if err != nil {
		return err
	}
	if *dryRun {
		encoder := json.NewEncoder(output)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(payload)
	}
	secret := routes[channel].secret
	if strings.TrimSpace(getenv(secret)) == "" {
		return fmt.Errorf("%s secret이 등록되지 않았습니다", secret)
	}
	target, err := webhookURL(getenv(secret))
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errNotification
	}
	if err := send(&http.Client{Timeout: 20 * time.Second}, target, body); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "Discord %s 채널에 태그 알림을 보냈습니다\n", channel)
	return err
}

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "::warning::"+err.Error())
		os.Exit(1)
	}
}
