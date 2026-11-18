// verifytools checks repository maintenance sources and statement coverage for CI.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var interpreter = regexp.MustCompile(`(?:^|[\s;|&'"])python(?:[0-9]+(?:\.[0-9]+)*)?(?:\.exe)?(?:\s|['"]|$)`)

func sources(root string) error {
	var violations []error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (name == "vendor" || name == "node_modules" || name == "__pycache__" || (strings.HasPrefix(name, ".") && name != ".github")) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.EqualFold(filepath.Ext(path), ".py") {
			violations = append(violations, fmt.Errorf("maintenance sources must use Go: %s", relative))
		}
		if strings.HasPrefix(filepath.ToSlash(relative), ".github/") && (filepath.Ext(path) == ".yml" || filepath.Ext(path) == ".yaml") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(data), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "#") && interpreter.MatchString(line) {
					violations = append(violations, fmt.Errorf("CI still invokes Python: %s", relative))
					break
				}
			}
		}
		return nil
	})
	return errors.Join(append(violations, err)...)
}

func coverage(input io.Reader, minimum float64) (float64, error) {
	if math.IsNaN(minimum) || math.IsInf(minimum, 0) || minimum < 0 || minimum > 100 {
		return 0, errors.New("minimum coverage must be between 0 and 100")
	}
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() {
		return 0, errors.Join(errors.New("coverage profile is empty"), scanner.Err())
	}
	if mode := scanner.Text(); mode != "mode: atomic" && mode != "mode: count" && mode != "mode: set" {
		return 0, errors.New("invalid coverage profile mode")
	}
	type block struct {
		statements uint64
		hit        bool
	}
	blocks := make(map[string]block)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return 0, errors.New("invalid coverage profile row")
		}
		statements, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		count, err := strconv.ParseUint(fields[2], 10, 64)
		if err != nil {
			return 0, err
		}
		previous, exists := blocks[fields[0]]
		if exists && previous.statements != statements {
			return 0, errors.New("inconsistent statement count for coverage block")
		}
		blocks[fields[0]] = block{statements, previous.hit || count > 0}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	var total, covered uint64
	for _, block := range blocks {
		if math.MaxUint64-total < block.statements {
			return 0, errors.New("coverage statement count overflow")
		}
		total += block.statements
		if block.hit {
			covered += block.statements
		}
	}
	if total == 0 {
		return 0, errors.New("coverage profile has no statements")
	}
	percent := float64(covered) / float64(total) * 100
	if percent < minimum {
		return percent, fmt.Errorf("statement coverage %.2f%% is below %.2f%%", percent, minimum)
	}
	return percent, nil
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("verifytools", flag.ContinueOnError)
	flags.SetOutput(output)
	root := flags.String("root", ".", "repository root")
	profile := flags.String("coverage", "", "optional Go coverage profile")
	minimum := flags.Float64("minimum", 80, "minimum statement coverage percentage")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if err := sources(*root); err != nil {
		return err
	}
	if *profile != "" {
		file, err := os.Open(*profile)
		if err != nil {
			return err
		}
		percent, checkErr := coverage(file, *minimum)
		if err := errors.Join(checkErr, file.Close()); err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "Statement coverage: %.2f%% (minimum %.2f%%)\n", percent, *minimum)
		return err
	}
	_, err := fmt.Fprintln(output, "Go maintenance sources and CI entrypoints verified")
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
