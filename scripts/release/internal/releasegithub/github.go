// Package releasegithub reads release evidence using the authenticated gh CLI.
package releasegithub

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
)

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var tagName = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-(preview|rc)\.[1-9][0-9]*)?$`)

type Client struct {
	Repository string
	token      string
	command    func([]string, []byte) ([]byte, string, error)
}

func New(repository, token string) (*Client, error) {
	if !repositoryName.MatchString(repository) {
		return nil, errors.New("invalid repository name")
	}
	client := &Client{Repository: repository, token: token}
	client.command = client.gh
	return client, nil
}

func (c *Client) gh(args []string, input []byte) ([]byte, string, error) {
	command := exec.Command("gh", args...)
	if c.token != "" {
		command.Env = append(command.Environ(), "GH_TOKEN="+c.token)
	}
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	result, err := command.Output()
	return result, strings.TrimSpace(stderr.String()), err
}

// API returns false only for an allowed 404. Authentication and transport errors propagate.
func (c *Client) API(method, path string, input, output any, allowMissing bool) (bool, error) {
	endpoint := "repos/" + c.Repository
	if path != "" {
		endpoint += "/" + path
	}
	args := []string{"api", "--method", method, endpoint}
	var data []byte
	if input != nil {
		var err error
		data, err = json.Marshal(input)
		if err != nil {
			return false, err
		}
		args = append(args, "--input", "-")
	}
	result, stderr, err := c.command(args, data)
	if err != nil {
		var body struct{ Status json.RawMessage }
		if allowMissing && json.Unmarshal(result, &body) == nil && (string(body.Status) == "404" || string(body.Status) == `"404"`) {
			return false, nil
		}
		return false, fmt.Errorf("GitHub API failure (%s): %s: %w", path, stderr, err)
	}
	if output != nil && len(bytes.TrimSpace(result)) != 0 {
		if err := json.Unmarshal(result, output); err != nil {
			return false, fmt.Errorf("decode GitHub API response (%s): %w", path, err)
		}
	}
	return true, nil
}

func (c *Client) Releases() ([]json.RawMessage, error) {
	var result []json.RawMessage
	for page := 1; page <= 100; page++ {
		var entries []json.RawMessage
		if _, err := c.API("GET", fmt.Sprintf("releases?per_page=100&page=%d", page), nil, &entries, false); err != nil {
			return nil, err
		}
		result = append(result, entries...)
		if len(entries) < 100 {
			return result, nil
		}
	}
	return nil, errors.New("the release list is too large to inspect completely")
}

func (c *Client) TagSHA(tag string) (string, error) {
	if !tagName.MatchString(tag) {
		return "", errors.New("invalid release tag")
	}
	var ref struct{ Object struct{ Type, SHA string } }
	found, err := c.API("GET", "git/ref/tags/"+tag, nil, &ref, true)
	if err != nil || !found {
		return "", err
	}
	for range 5 {
		switch ref.Object.Type {
		case "commit":
			if ref.Object.SHA == "" {
				return "", errors.New("tag commit SHA is missing")
			}
			return ref.Object.SHA, nil
		case "tag":
			var next = ref
			next.Object.Type, next.Object.SHA = "", ""
			if _, err := c.API("GET", "git/tags/"+ref.Object.SHA, nil, &next, false); err != nil {
				return "", err
			}
			ref = next
		default:
			return "", errors.New("the tag does not resolve to a single commit")
		}
	}
	return "", errors.New("the tag does not resolve to a single commit")
}

type Artifact struct {
	ID          int64
	Name        string
	Expired     *bool
	Digest      string
	CreatedAt   string `json:"created_at"`
	WorkflowRun struct {
		ID         int64
		HeadSHA    string `json:"head_sha"`
		HeadBranch string `json:"head_branch"`
	} `json:"workflow_run"`
}

type Evidence struct {
	Artifact    Artifact        `json:"artifact"`
	WorkflowRun json.RawMessage `json:"workflow_run"`
	Manifest    json.RawMessage `json:"manifest"`
}

func (c *Client) CandidateEvidence(tag, prefix string, allowAbsent bool) (*Evidence, error) {
	if !tagName.MatchString(tag) {
		return nil, errors.New("invalid release tag")
	}
	name := prefix + tag
	var candidates []Artifact
	for page := 1; page <= 100; page++ {
		var result struct{ Artifacts []Artifact }
		path := fmt.Sprintf("actions/artifacts?name=%s&per_page=100&page=%d", name, page)
		if _, err := c.API("GET", path, nil, &result, false); err != nil {
			return nil, err
		}
		for _, item := range result.Artifacts {
			if item.Name == name && (!allowAbsent || item.Expired != nil && !*item.Expired) {
				candidates = append(candidates, item)
			}
		}
		if len(result.Artifacts) < 100 {
			break
		}
		if page == 100 {
			return nil, errors.New("failed to inspect all candidate verification records")
		}
	}
	if allowAbsent && len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) != 1 || candidates[0].Expired == nil || *candidates[0].Expired {
		return nil, errors.New("the verification artifact for the tag is missing, duplicated or expired")
	}
	artifact := candidates[0]
	var run json.RawMessage
	if _, err := c.API("GET", fmt.Sprintf("actions/runs/%d", artifact.WorkflowRun.ID), nil, &run, false); err != nil {
		return nil, err
	}
	endpoint := fmt.Sprintf("repos/%s/actions/artifacts/%d/zip", c.Repository, artifact.ID)
	data, stderr, err := c.command([]string{"api", endpoint}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download candidate verification artifact: %s: %w", stderr, err)
	}
	if artifact.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256(data)) {
		return nil, errors.New("candidate verification artifact SHA-256 does not match")
	}
	manifest, err := readManifest(data)
	if err != nil {
		return nil, fmt.Errorf("candidate verification artifact has no valid manifest: %w", err)
	}
	return &Evidence{artifact, run, manifest}, nil
}

func readManifest(data []byte) (json.RawMessage, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	file, err := archive.Open("release-manifest.json")
	if err != nil {
		return nil, err
	}
	content, err := io.ReadAll(file)
	if err := errors.Join(err, file.Close()); err != nil {
		return nil, err
	}
	if !json.Valid(content) {
		return nil, errors.New("invalid JSON")
	}
	return content, nil
}
