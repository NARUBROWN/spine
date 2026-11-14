package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const registry = "https://registry.npmjs.org/"

var noticePattern = regexp.MustCompile(`(?i)(^|/)(licen[cs]e|copying|copyright|notice)(\.|$|-)`)

type lockEntry struct {
	Version   string
	License   string
	Resolved  string
	Integrity string
	Dev       bool
}

func download(address string) ([]byte, error) {
	if !strings.HasPrefix(address, registry) {
		return nil, fmt.Errorf("Distribution source is not allowed: %s", address)
	}
	client := http.Client{Timeout: 60 * time.Second}
	client.CheckRedirect = func(req *http.Request, previous []*http.Request) error {
		if !strings.HasPrefix(req.URL.String(), registry) {
			return errors.New("Distribution source changed")
		}
		if len(previous) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "spine-openapi-ui-updater")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", address, response.StatusCode)
	}
	return io.ReadAll(response.Body)
}

func (u updater) archive(entry lockEntry) (map[string][]byte, error) {
	path := filepath.Join(u.cache, digest([]byte(entry.Integrity)))
	data, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if missing {
		data, err = u.download(entry.Resolved)
	}
	if err != nil {
		return nil, err
	}
	sum := sha512.Sum512(data)
	if entry.Integrity != "sha512-"+base64.StdEncoding.EncodeToString(sum[:]) {
		return nil, fmt.Errorf("Distribution integrity verification failed: %s", entry.Resolved)
	}
	if missing {
		if err := os.MkdirAll(u.cache, 0755); err != nil {
			return nil, err
		}
		temporary, err := os.CreateTemp(u.cache, "download-")
		if err != nil {
			return nil, err
		}
		defer os.Remove(temporary.Name())
		_, writeErr := temporary.Write(data)
		if err := errors.Join(writeErr, temporary.Close()); err != nil {
			return nil, err
		}
		if err := os.Rename(temporary.Name(), path); err != nil {
			return nil, err
		}
	}
	stream, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	archive := tar.NewReader(stream)
	result := make(map[string][]byte)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		if !header.FileInfo().Mode().IsRegular() {
			continue
		}
		_, relative, found := strings.Cut(header.Name, "/")
		if found && (!strings.Contains(relative, "/") || strings.HasPrefix(relative, "bundles/") || noticePattern.MatchString(relative)) {
			result[relative], err = io.ReadAll(archive)
			if err != nil {
				return nil, err
			}
		}
	}
}
