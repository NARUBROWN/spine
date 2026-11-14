package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const toolsPath = "scripts/openapi-ui"

var distributions = []struct {
	name            string
	major           int
	target, license string
	assets          []string
}{
	{"swagger-ui-dist", 5, "swaggerui", "Apache-2.0", []string{"swagger-ui-bundle.js", "swagger-ui.css", "LICENSE", "NOTICE", "swagger-ui-bundle.js.LICENSE.txt"}},
	{"redoc", 2, "redoc", "MIT", []string{"bundles/redoc.standalone.js", "LICENSE"}},
}

type dependencies struct {
	Dependencies map[string]string `json:"dependencies"`
}

type distribution struct {
	Version   string `json:"version"`
	License   string `json:"license"`
	Tarball   string `json:"tarball"`
	Integrity string `json:"integrity"`
}

type dependency struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	License   string `json:"license"`
	Integrity string `json:"integrity"`
}

type manifest struct {
	Format        int                     `json:"format"`
	Distributions map[string]distribution `json:"distributions"`
	Dependencies  []dependency            `json:"dependencies"`
	Files         map[string]string       `json:"files"`
	LocalPatches  map[string]string       `json:"local_patches"`
}

type updater struct {
	root     string
	cache    string
	download func(string) ([]byte, error)
}

func (u updater) readJSON(name string, value any) error {
	data, err := os.ReadFile(filepath.Join(u.root, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func marshal(value any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value)
	return b.Bytes(), err
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

var stablePattern = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)

func stable(value string) ([3]uint64, bool) {
	var parts [3]uint64
	match := stablePattern.FindStringSubmatch(value)
	if match == nil {
		return parts, false
	}
	for i := range parts {
		var err error
		parts[i], err = strconv.ParseUint(match[i+1], 10, 64)
		if err != nil {
			return parts, false
		}
	}
	return parts, true
}

func (u updater) check() error {
	var pins dependencies
	var m manifest
	if err := u.readJSON(toolsPath+"/manifest.json", &m); err != nil {
		return err
	}
	if err := u.readJSON(toolsPath+"/package.json", &pins); err != nil {
		return err
	}
	for name, item := range m.Distributions {
		valid := false
		version, ok := stable(item.Version)
		for _, distro := range distributions {
			if distro.name == name && ok && version[0] == uint64(distro.major) && pins.Dependencies[name] == item.Version {
				valid = true
			}
		}
		if !valid {
			return fmt.Errorf("Pinned version does not match: %s", name)
		}
	}
	for name, hash := range m.Files {
		data, err := os.ReadFile(filepath.Join(u.root, name))
		if err != nil {
			return err
		}
		if digest(data) != hash {
			return fmt.Errorf("Embedded file changed: %s", name)
		}
	}
	fmt.Printf("Verified %d asset, notice and input files.\n", len(m.Files))
	return nil
}

type upstreamInfo struct {
	Pinned          string `json:"pinned"`
	Supported       string `json:"supported"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
}

func (u updater) upstream(pins dependencies) (map[string]upstreamInfo, error) {
	report := make(map[string]upstreamInfo)
	for _, distro := range distributions {
		data, err := u.download(registry + distro.name)
		if err != nil {
			return nil, err
		}
		var metadata struct {
			Versions map[string]json.RawMessage
			Tags     map[string]string `json:"dist-tags"`
		}
		if err := json.Unmarshal(data, &metadata); err != nil {
			return nil, err
		}
		var best [3]uint64
		var selected string
		for version := range metadata.Versions {
			parts, ok := stable(version)
			if ok && parts[0] == uint64(distro.major) && (selected == "" || parts[1] > best[1] || (parts[1] == best[1] && parts[2] > best[2])) {
				best, selected = parts, version
			}
		}
		if selected == "" {
			return nil, fmt.Errorf("no supported version for %s", distro.name)
		}
		pin := pins.Dependencies[distro.name]
		report[distro.name] = upstreamInfo{pin, selected, metadata.Tags["latest"], pin != selected}
	}
	return report, nil
}

func run() error {
	check := flag.Bool("check", false, "Verify pinned versions and file hashes without network access")
	checkUpstream := flag.Bool("check-upstream", false, "Check supported and latest major versions")
	refresh := flag.Bool("refresh", false, "Reproduce pinned distributions and notices")
	update := flag.Bool("update", false, "Update stable versions within supported major versions")
	flag.Parse()
	count := 0
	for _, option := range []*bool{check, checkUpstream, refresh, update} {
		if *option {
			count++
		}
	}
	if flag.NArg() != 0 || count != 1 {
		return errors.New("specify exactly one of --check, --check-upstream, --refresh or --update")
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	u := updater{root, filepath.Join(os.TempDir(), "spine-openapi-ui-cache"), download}
	if *check {
		return u.check()
	}
	if *checkUpstream {
		var pins dependencies
		if err := u.readJSON(toolsPath+"/package.json", &pins); err != nil {
			return err
		}
		report, err := u.upstream(pins)
		if err != nil {
			return err
		}
		data, err := marshal(report)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err
	}
	return u.refresh(*update)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
