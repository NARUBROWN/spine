package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/NARUBROWN/spine/scripts/release/internal/publiccheck"
)

func run() error {
	planPath := flag.String("plan", "", "release plan")
	scope := flag.String("scope", "", "core, platform or cohort")
	source := flag.String("source", "", "source checkout")
	query := flag.String("query", "", "public module query")
	output := flag.String("output", "", "verification report path")
	flag.Parse()
	if *planPath == "" || *scope == "" || *source == "" || *query == "" || *output == "" || flag.NArg() != 0 {
		return errors.New("--plan, --scope, --source, --query and --output are required")
	}
	data, err := os.ReadFile(*planPath)
	if err != nil {
		return err
	}
	var plan publiccheck.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return err
	}
	root, err := filepath.Abs(*source)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "spine-public-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	check := publiccheck.NewRunner(publiccheck.Environment(work), os.Stdout)
	report, err := check.Verify(plan, root, work, *query, *scope)
	if err != nil {
		return err
	}
	result, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(*output, append(result, '\n'), 0644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
