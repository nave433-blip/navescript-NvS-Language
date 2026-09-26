package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Wave 17: `nvs test` — discovers test_*.nvs / *_test.nvs files and runs
// each in a fresh interpreter subprocess (clean global state per file,
// honest isolation). A file passes when it exits 0; `assert` failures
// and runtime errors exit nonzero by construction.

// TestResult is the outcome of running one test file.
type TestResult struct {
	File     string
	Passed   bool
	Duration time.Duration
	Output   string // captured combined output (kept short on success)
}

// DiscoverTests walks dirs (default ".") and returns test files:
// test_*.nvs, test_*.ns, *_test.nvs, *_test.ns. Hidden directories and
// common build/vendor dirs are skipped.
func DiscoverTests(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	var files []string
	isTest := func(name string) bool {
		base := filepath.Base(name)
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".nvs" && ext != ".ns" {
			return false
		}
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		return strings.HasPrefix(stem, "test_") || strings.HasSuffix(stem, "_test")
	}
	for _, dir := range dirs {
		root := dir
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip unreadable entries, don't abort the run
			}
			if info.IsDir() {
				if path == root {
					return nil // never skip the walk root itself (e.g. ".")
				}
				base := filepath.Base(path)
				if strings.HasPrefix(base, ".") || base == "bin" || base == "vendor" || base == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if isTest(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// NvsBin overrides the interpreter binary used to run test files.
// Empty means os.Executable(), which is correct when invoked as
// `nvs test` (the running binary IS nvs). Tests set it to a stub.
var NvsBin string

// RunTestFile executes one test file in a subprocess of the current nvs
// binary with a timeout. It never panics; failures are data.
func RunTestFile(path string, timeout time.Duration, verbose bool, out io.Writer) TestResult {
	start := time.Now()
	res := TestResult{File: path}
	exe := NvsBin
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			res.Output = "cannot locate nvs binary: " + err.Error()
			return res
		}
	}
	var buf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "run", path)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	timedOut := ctx.Err() == context.DeadlineExceeded
	res.Duration = time.Since(start)
	res.Passed = runErr == nil && !timedOut
	if timedOut {
		buf.WriteString(fmt.Sprintf("\n[timeout after %s]", timeout))
	}
	res.Output = buf.String()
	if verbose {
		fmt.Fprintf(out, "--- %s (%s)\n%s", path, res.Duration.Round(time.Millisecond), res.Output)
	}
	return res
}

// RunTestSuite discovers and runs all tests, printing a summary.
// Returns the number of failures.
func RunTestSuite(dirs []string, timeout time.Duration, verbose bool, out io.Writer) int {
	files, err := DiscoverTests(dirs)
	if err != nil {
		fmt.Fprintf(out, "nvs test: discovery error: %v\n", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintln(out, "nvs test: no test files found (looking for test_*.nvs / *_test.nvs)")
		return 0
	}
	fmt.Fprintf(out, "nvs test: %d file(s)\n", len(files))
	failures := 0
	for _, f := range files {
		res := RunTestFile(f, timeout, verbose, out)
		status := "ok  "
		if !res.Passed {
			status = "FAIL"
			failures++
		}
		fmt.Fprintf(out, "%s  %s (%s)\n", status, res.File, res.Duration.Round(time.Millisecond))
		if !res.Passed && !verbose {
			for _, line := range strings.Split(res.Output, "\n") {
				fmt.Fprintf(out, "      | %s\n", line)
			}
		}
	}
	fmt.Fprintf(out, "\n%d passed, %d failed\n", len(files)-failures, failures)
	return failures
}
