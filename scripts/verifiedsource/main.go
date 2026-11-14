package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

func verify(results, sha string, output io.Writer) error {
	var jobs map[string]struct {
		Result string
	}
	if err := json.Unmarshal([]byte(results), &jobs); err != nil {
		return err
	}
	if jobs == nil {
		return errors.New("invalid verification results")
	}
	for _, job := range jobs {
		if job.Result != "success" {
			return errors.New("All required CI jobs must succeed")
		}
	}
	_, err := fmt.Fprintf(output, "sha=%s\n", sha)
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return errors.New("GITHUB_OUTPUT is missing")
	}
	// Verify before opening the output so a failed job cannot emit a source SHA.
	var output bytes.Buffer
	if err := verify(os.Getenv("RESULTS"), os.Getenv("SOURCE_SHA"), &output); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	_, err = file.Write(output.Bytes())
	return errors.Join(err, file.Close())
}
