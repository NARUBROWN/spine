package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"time"

	"github.com/NARUBROWN/spine/scripts/release/internal/approval"
	"github.com/NARUBROWN/spine/scripts/release/internal/releasegithub"
)

func publish(repo planningRepo, approved, raw document, verified, ref string, replan func(string, string, string, string) (document, error)) error {
	if verified == "" || verified != textField(approved, "source_sha") {
		return errors.New("The verified commit does not match the publication target")
	}
	branch := textField(approved, "branch")
	if ref != "refs/heads/"+branch {
		return errors.New("The release workflow branch does not match the channel")
	}
	current, err := policyAt(repo, branch)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, raw) {
		return errors.New("The channel branch release policy changed after the plan was created")
	}
	source := textField(approved, "source_tag")
	if source == "" {
		source = verified
	}
	version := textField(approved, "version")
	fresh, err := replan(textField(approved, "channel"), version, source, textField(approved, "observation_override_reason"))
	if err != nil {
		return err
	}
	// Compare both manifests using the same JSON number representation.
	encoded, err := json.Marshal(fresh)
	if err != nil {
		return err
	}
	fresh, err = decodeDocument(encoded)
	if err != nil {
		return err
	}
	for _, key := range []string{"source_sha", "source_tree", "source_fingerprint", "kind", "module", "branch", "promotion_pr", "source_evidence", "channel", "version", "components"} {
		if !reflect.DeepEqual(fresh[key], approved[key]) {
			return errors.New("The approved plan changed. Start a new run.")
		}
	}
	_, err = repo.API("POST", "git/refs", document{"ref": "refs/tags/" + version, "sha": verified}, nil, false)
	return err
}

func writePlan(path string, doc document) error {
	var data bytes.Buffer
	enc := json.NewEncoder(&data)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return os.WriteFile(path, data.Bytes(), 0644)
}

func runPublication(command, root, path string, p policy, data []byte) error {
	raw, err := decodeDocument(data)
	if err != nil {
		return err
	}
	repository := os.Getenv("GITHUB_REPOSITORY")
	repo, err := releasegithub.New(repository, "")
	if err != nil {
		return err
	}
	attempt, ok := os.LookupEnv("GITHUB_RUN_ATTEMPT")
	if !ok {
		attempt = "1"
	}
	replan := func(channel, version, source, reason string) (document, error) {
		return makePlan(repo, repository, p, raw, channel, version, source, reason, os.Getenv("GITHUB_RUN_ID"), attempt, time.Now(), func() error {
			return approval.Verify(repo, p.Environment, p.Reviewer)
		})
	}
	ref := os.Getenv("GITHUB_REF")
	if command == "tag" {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		approved, err := decodeDocument(data)
		if err != nil {
			return err
		}
		return publish(repo, approved, raw, os.Getenv("VERIFIED_SHA"), ref, replan)
	}
	channel := os.Getenv("RELEASE_CHANNEL")
	branch, ok := p.Branches[channel]
	if !ok || ref != "refs/heads/"+branch {
		return errors.New("Run the workflow from the preview, rc or main branch corresponding to the channel")
	}
	source, version := os.Getenv("RELEASE_SOURCE"), os.Getenv("RELEASE_VERSION")
	if channel == "stable" {
		parsed := tag.FindStringSubmatch(source)
		if parsed == nil {
			return errors.New("Version must use the vX.Y.Z-preview.N, vX.Y.Z-rc.N or vX.Y.Z format")
		}
		version = parsed[1]
	}
	if err := checkModule(root, p, channel); err != nil {
		return err
	}
	if p.Kind == "core" && version != p.Channels[channel].Core {
		return errors.New("The core candidate tag does not match the core tag in the channel component versions")
	}
	doc, err := replan(channel, version, source, os.Getenv("RELEASE_OVERRIDE_REASON"))
	if err != nil {
		return err
	}
	if err := writePlan(path, doc); err != nil {
		return err
	}
	return writePublicationOutputs(doc, os.Getenv("GITHUB_OUTPUT"), os.Getenv("GITHUB_STEP_SUMMARY"))
}

func writePublicationOutputs(doc document, output, summary string) error {
	target := textField(doc, "source_sha")
	version := textField(doc, "version")
	if output != "" {
		if err := appendFile(output, func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "sha=%s\nversion=%s\nkind=%s\n", target, version, textField(doc, "kind"))
			return err
		}); err != nil {
			return err
		}
	}
	if summary != "" {
		reason := textField(doc, "observation_override_reason")
		if reason == "" {
			reason = "none"
		}
		return appendFile(summary, func(w io.Writer) error {
			_, err := fmt.Fprintf(w, "Release plan: `%s` / `%s`\n\nObservation period override reason: %s\n", version, target, reason)
			return err
		})
	}
	return nil
}
