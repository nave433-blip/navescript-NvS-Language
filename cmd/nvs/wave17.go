package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/tools"
)

// Wave 17 tooling commands: pkg, lsp, debug, test, run --watch, run --profile.

func runTestCmd(args []string) {
	verbose := false
	var dirs []string
	timeout := 30 * time.Second
	for _, a := range args {
		switch {
		case a == "-v" || a == "--verbose":
			verbose = true
		case strings.HasPrefix(a, "--timeout="):
			d, err := time.ParseDuration(strings.TrimPrefix(a, "--timeout="))
			if err != nil {
				fmt.Fprintf(os.Stderr, "nvs test: bad --timeout value: %v\n", err)
				os.Exit(1)
			}
			timeout = d
		case a == "-h" || a == "--help":
			fmt.Println(`nvs test — run test_*.nvs / *_test.nvs files

Usage:
  nvs test [dir...] [--timeout=30s] [-v]

Each test file runs in a fresh interpreter subprocess; exit 0 passes.
Uses assert(cond, msg) or any runtime error to fail.`)
			return
		default:
			dirs = append(dirs, a)
		}
	}
	if failures := tools.RunTestSuite(dirs, timeout, verbose, os.Stdout); failures > 0 {
		os.Exit(1)
	}
}

// runWatch re-runs a program whenever it (or nearby .nvs/.ns files)
// changes. Polling-based: no new dependencies, works everywhere.
func runWatch(path string) {
	dir := filepath.Dir(path)
	absDir, _ := filepath.Abs(dir)
	fmt.Printf("nvs watch: %s (Ctrl+C to stop)\n", path)
	last := watchSnapshot(absDir)
	runOnce(path)
	for {
		time.Sleep(500 * time.Millisecond)
		cur := watchSnapshot(absDir)
		if !mapsEqual(last, cur) {
			last = cur
			fmt.Printf("\n--- changed; re-running %s ---\n", path)
			runOnce(path)
		}
	}
}

func watchSnapshot(dir string) map[string]time.Time {
	snap := map[string]time.Time{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return snap
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".nvs") || strings.HasSuffix(name, ".ns") {
			if info, err := e.Info(); err == nil {
				snap[name] = info.ModTime()
			}
		}
	}
	return snap
}

func mapsEqual(a, b map[string]time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || !w.Equal(v) {
			return false
		}
	}
	return true
}

func runOnce(path string) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		return
	}
	cmd := exec.Command(exe, "run", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("[exit %d]\n", exitErr.ExitCode())
		} else {
			fmt.Printf("[error: %v]\n", err)
		}
	}
}

func runProfile(path string, top int) {
	if err := tools.ProfileFile(path, top, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
