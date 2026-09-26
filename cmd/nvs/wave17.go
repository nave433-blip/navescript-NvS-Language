package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/debug"
	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lsp"
	"github.com/navescript/nvs/internal/pkg"
	"github.com/navescript/nvs/internal/tools"
)

// WirePackageManager installs the package resolver into the evaluator so
// `import "pkgname"` resolves from the nvs package cache.
func WirePackageManager() {
	eval.PackageResolver = pkg.Resolve
}

// runPkgCmd implements `nvs pkg <subcommand>`. Honest scope: GitHub is
// the registry — there is no central NvS package server.
func runPkgCmd(args []string) {
	if len(args) == 0 {
		pkgHelp()
		return
	}
	switch args[0] {
	case "init":
		dir := "."
		if len(args) > 1 {
			dir = args[1]
		}
		m, err := pkg.Init(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nvs pkg init: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("initialized NvS package %q in %s\n", m.Name, dir)
	case "install", "add":
		if len(args) < 2 {
			// No spec: replay the lockfile, like the docs promise.
			if err := pkg.InstallFromLock("."); err != nil {
				fmt.Fprintf(os.Stderr, "nvs pkg install: %v\n", err)
				fmt.Fprintln(os.Stderr, "usage: nvs pkg install <user/repo[@tag]|./path|/abs/path>")
				os.Exit(1)
			}
			fmt.Println("installed all packages from nvs.lock")
			break
		}
		for _, spec := range args[1:] {
			inst, err := pkg.Install(spec, pkg.InstallOptions{})
			if err != nil {
				fmt.Fprintf(os.Stderr, "nvs pkg install %s: %v\n", spec, err)
				os.Exit(1)
			}
			fmt.Printf("installed %s@%s → %s\n", inst.Name, inst.Version, inst.Dir)
		}
	case "list", "ls":
		pkgs, err := pkg.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "nvs pkg list: %v\n", err)
			os.Exit(1)
		}
		if len(pkgs) == 0 {
			fmt.Println("no packages installed")
			return
		}
		for _, p := range pkgs {
			fmt.Printf("%s@%s  (%s)\n", p.Name, p.Version, p.Source)
		}
	case "remove", "rm", "uninstall":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: nvs pkg remove <name>")
			os.Exit(1)
		}
		if err := pkg.Remove(args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "nvs pkg remove: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed %s\n", args[1])
	case "publish":
		steps, err := pkg.PublishCheck(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "nvs pkg publish: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(steps)
	case "-h", "--help", "help":
		pkgHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown pkg subcommand: %s\n", args[0])
		pkgHelp()
		os.Exit(1)
	}
}

func pkgHelp() {
	fmt.Print(`nvs pkg — package manager (GitHub is the registry; no central server)

Usage:
  nvs pkg init [dir]        Scaffold nvs.json in dir (default .)
  nvs pkg install <spec>    Install a package (alias: nvs get <spec>)
                            spec: user/repo, user/repo@tag, ./path, /abs/path
  nvs pkg list              List installed packages
  nvs pkg remove <name>     Remove a package
  nvs pkg publish           Validate and print manual publish steps

Installed packages live in ~/.nvs/packages; import "name" resolves
through the cache. Installs are recorded in ./nvs.lock.
`)
}

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

// runLspCmd starts the Language Server Protocol server on stdio.
func runLspCmd() {
	lsp.Serve(os.Stdin, os.Stdout)
}

// runDebugCmd starts an interactive debugging session for a program.
func runDebugCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nvs debug <file (.ns or .nvs)>")
		os.Exit(1)
	}
	sess := debug.New(os.Stdin, os.Stdout)
	if err := sess.RunFile(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

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
