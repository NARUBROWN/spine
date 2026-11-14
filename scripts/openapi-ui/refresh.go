package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"
)

type swaggerPatches struct {
	Version        string
	UpstreamSHA256 string `json:"upstream_sha256"`
	Notice         string
	Replacements   []struct{ Issue, Before, After string }
}

func patchSwagger(data []byte, version string, patches swaggerPatches) ([]byte, error) {
	if version != patches.Version {
		return nil, fmt.Errorf("Local Swagger UI fixes require review for version %s", version)
	}
	if digest(data) != patches.UpstreamSHA256 {
		return nil, errors.New("Local Swagger UI fixes require the reviewed upstream bundle")
	}
	for _, patch := range patches.Replacements {
		before, after := []byte(patch.Before), []byte(patch.After)
		if bytes.Count(data, before) != 1 {
			return nil, fmt.Errorf("Local Swagger UI fix no longer matches: %s", patch.Issue)
		}
		data = bytes.Replace(data, before, after, 1)
	}
	return append([]byte(patches.Notice), data...), nil
}

func (u updater) notices(entries map[string]lockEntry) ([]notice, error) {
	var keys []string
	for key, entry := range entries {
		if key != "" && !entry.Dev {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	fmt.Printf("Checking dependency licenses: %d packages\n", len(keys))
	results := make([]notice, len(keys))
	errs := make([]error, len(keys))
	jobs := make(chan int)
	var group sync.WaitGroup
	for range min(12, len(keys)) {
		group.Go(func() {
			for index := range jobs {
				key := keys[index]
				results[index], errs[index] = u.dependencyNotice(key, entries[key])
			}
		})
	}
	for index := range keys {
		jobs <- index
	}
	close(jobs)
	group.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	var previous manifest
	if err := u.readJSON(toolsPath+"/manifest.json", &previous); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	licenses := make(map[string]map[string]bool)
	for _, dep := range previous.Dependencies {
		if licenses[dep.Name] == nil {
			licenses[dep.Name] = make(map[string]bool)
		}
		licenses[dep.Name][dep.License] = true
	}
	for _, result := range results {
		dep := result.info
		if known := licenses[dep.Name]; known != nil && !known[dep.License] {
			return nil, fmt.Errorf("License change review required: %s (%s)", dep.Name, dep.License)
		}
	}
	return results, nil
}

func (u updater) bundles(entries map[string]lockEntry, licenses string, files map[string][]byte) (map[string]distribution, error) {
	result := make(map[string]distribution)
	for _, distro := range distributions {
		entry := entries["node_modules/"+distro.name]
		if entry.License != distro.license {
			return nil, fmt.Errorf("Upstream license changed: %s", distro.name)
		}
		contents, err := u.archive(entry)
		if err != nil {
			return nil, err
		}
		if distro.name == "swagger-ui-dist" {
			var patches swaggerPatches
			if err := u.readJSON(toolsPath+"/swagger-ui-fixes.json", &patches); err != nil {
				return nil, err
			}
			contents["swagger-ui-bundle.js"], err = patchSwagger(contents["swagger-ui-bundle.js"], entry.Version, patches)
			if err != nil {
				return nil, err
			}
		}
		if distro.name == "redoc" && bytes.Count(contents["bundles/redoc.standalone.js"], []byte("https://cdn.redoc.ly/redoc/logo-mini.svg")) != 1 {
			return nil, errors.New("The offline handling of the ReDoc logo requires review")
		}
		var additional []string
		for key := range contents {
			if strings.HasSuffix(key, ".LICENSE.txt") && strings.Contains(key, "redoc.standalone") {
				additional = append(additional, key)
			}
		}
		slices.Sort(additional)
		assets := append(slices.Clone(distro.assets), additional...)
		var exact []string
		target := "pkg/openapi/ui/" + distro.target + "/assets/"
		for _, source := range assets {
			data, ok := contents[source]
			if !ok {
				return nil, fmt.Errorf("Distribution file is missing: %s/%s", distro.name, source)
			}
			files[target+path.Base(source)] = data
			if strings.HasSuffix(source, ".LICENSE.txt") {
				if !utf8.Valid(data) {
					return nil, fmt.Errorf("invalid UTF-8 notice: %s", source)
				}
				exact = append(exact, string(data))
			}
		}
		files[target+"THIRD_PARTY_LICENSES.txt"] = []byte("Original upstream bundle notices:\n" + strings.Join(exact, "\n") + "\n" + licenses)
		result[distro.name] = distribution{entry.Version, entry.License, entry.Resolved, entry.Integrity}
	}
	return result, nil
}

func (u updater) refresh(update bool) error {
	packagePath := filepath.Join(u.root, toolsPath, "package.json")
	packageData, err := os.ReadFile(packagePath)
	if err != nil {
		return err
	}
	if update {
		var pins dependencies
		if err := json.Unmarshal(packageData, &pins); err != nil {
			return err
		}
		upstream, err := u.upstream(pins)
		if err != nil {
			return err
		}
		for name, info := range upstream {
			pins.Dependencies[name] = info.Supported
			if name == "swagger-ui-dist" {
				pins.Dependencies["swagger-ui"] = info.Supported
			}
		}
		data, err := marshal(pins.Dependencies)
		if err != nil {
			return err
		}
		var metadata map[string]json.RawMessage
		if err := json.Unmarshal(packageData, &metadata); err != nil {
			return err
		}
		metadata["dependencies"] = data
		packageData, err = marshal(metadata)
		if err != nil {
			return err
		}
	}
	staging, err := os.MkdirTemp("", "spine-ui-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := os.WriteFile(filepath.Join(staging, "package.json"), packageData, 0644); err != nil {
		return err
	}
	lockName := "package-lock.json"
	lock, err := os.ReadFile(filepath.Join(u.root, toolsPath, lockName))
	if err == nil {
		if err := os.WriteFile(filepath.Join(staging, lockName), lock, 0644); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	npm := exec.Command("npm", "install", "--package-lock-only", "--ignore-scripts", "--omit=dev", "--no-audit", "--no-fund")
	npm.Dir, npm.Stdout, npm.Stderr = staging, os.Stdout, os.Stderr
	if err := npm.Run(); err != nil {
		return err
	}
	lock, err = os.ReadFile(filepath.Join(staging, lockName))
	if err != nil {
		return err
	}
	var parsed struct{ Packages map[string]lockEntry }
	if err := json.Unmarshal(lock, &parsed); err != nil {
		return err
	}
	notices, err := u.notices(parsed.Packages)
	if err != nil {
		return err
	}
	var inventory strings.Builder
	inventory.WriteString("Supplementary runtime dependency license inventory.\nResolved from the pinned source packages. This inventory is a conservative\nsuperset, not an exact bill of materials for the prebuilt JavaScript bundles.\nOriginal bundle notices and license comments are preserved separately.\n")
	m := manifest{Format: 1, Files: make(map[string]string), LocalPatches: map[string]string{"swagger-ui-dist": toolsPath + "/swagger-ui-fixes.json"}, Dependencies: make([]dependency, 0, len(notices))}
	for _, notice := range notices {
		inventory.WriteString(notice.text)
		m.Dependencies = append(m.Dependencies, notice.info)
	}
	files := map[string][]byte{toolsPath + "/package.json": packageData, toolsPath + "/" + lockName: lock}
	m.Distributions, err = u.bundles(parsed.Packages, inventory.String(), files)
	if err != nil {
		return err
	}
	inputs := []string{toolsPath + "/license-overrides.json", toolsPath + "/swagger-ui-fixes.json"}
	licenses, err := filepath.Glob(filepath.Join(u.root, toolsPath, "licenses", "*.txt"))
	if err != nil {
		return err
	}
	for _, license := range licenses {
		inputs = append(inputs, toolsPath+"/licenses/"+filepath.Base(license))
	}
	for _, name := range inputs {
		files[name], err = os.ReadFile(filepath.Join(u.root, name))
		if err != nil {
			return err
		}
	}
	for name, data := range files {
		m.Files[name] = digest(data)
	}
	// Keep the distribution order used by the generated manifest.
	payload, err := marshal(struct {
		Format        int `json:"format"`
		Distributions struct {
			Swagger distribution `json:"swagger-ui-dist"`
			Redoc   distribution `json:"redoc"`
		} `json:"distributions"`
		Dependencies []dependency      `json:"dependencies"`
		Files        map[string]string `json:"files"`
		LocalPatches map[string]string `json:"local_patches"`
	}{
		Format: m.Format,
		Distributions: struct {
			Swagger distribution `json:"swagger-ui-dist"`
			Redoc   distribution `json:"redoc"`
		}{m.Distributions["swagger-ui-dist"], m.Distributions["redoc"]},
		Dependencies: m.Dependencies,
		Files:        m.Files,
		LocalPatches: m.LocalPatches,
	})
	if err != nil {
		return err
	}
	for name, data := range files {
		target := filepath.Join(u.root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(u.root, toolsPath, "manifest.json"), payload, 0644); err != nil {
		return err
	}
	return u.check()
}
