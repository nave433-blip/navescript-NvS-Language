package main

// Wave 11 — polyglot tooling: `nvs exports` and `nvs bindgen`.

import (
	"fmt"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/tools"
)

func exportsHelp() {
	fmt.Print(`nvs exports — describe an NvS file's public surface as JSON

Usage:
  nvs exports <file.ns>
  nvs exports --help

Parses the file WITHOUT running it and prints a JSON document describing
the top-level bindings a foreign client can call: functions (names,
parameters, defaults, return annotations; generator functions are marked
"kind": "generator"), classes (methods), enums, and constants.

This is how non-NvS tooling "reads" NvS: feed it to nvs bindgen, or parse
the JSON in any language to build your own client.
`)
}

func runExports(args []string) {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			exportsHelp()
			return
		}
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: nvs exports <file.ns>")
		os.Exit(1)
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "exports: %v\n", err)
		os.Exit(1)
	}
	out, err := tools.ExportSourceJSON(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "exports: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func bindgenHelp() {
	fmt.Print(`nvs bindgen — generate a foreign-language client module for an NvS file

Usage:
  nvs bindgen --to=python <file.ns> -o <module>.py
  nvs bindgen --help

Reads the file's public surface (see nvs exports) and generates a module
in the target language that exposes every top-level NvS function as a
native callable. Calls are routed through the JSON stdio bridge
(nvs bridge): the generated module spawns NvS as a subprocess, evaluates
the embedded source once, and forwards each call.

Value mapping follows the bridge protocol (see docs/POLYGLOT.md):
int/float/string/bool/null/array/hash round-trip; anything else is an
honest error. NvS runtime errors surface as exceptions in the target
language (Python: NvSError).

Supported targets: python
`)
}

func runBindgen(args []string) {
	target := ""
	out := ""
	var rest []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			bindgenHelp()
			return
		case strings.HasPrefix(a, "--to="):
			target = strings.TrimPrefix(a, "--to=")
		case a == "-o":
			out = "-" // value comes next; handled below
		case strings.HasPrefix(a, "-o"):
			out = strings.TrimPrefix(a, "-o")
		case out == "-":
			out = a
		default:
			rest = append(rest, a)
		}
	}
	if target == "" || len(rest) != 1 || out == "" {
		fmt.Fprintln(os.Stderr, "usage: nvs bindgen --to=python <file.ns> -o <module>.py")
		os.Exit(1)
	}
	data, err := os.ReadFile(rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "bindgen: %v\n", err)
		os.Exit(1)
	}
	gen, err := tools.Bindgen(target, rest[0], string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "bindgen: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(gen), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "bindgen: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "bindgen: wrote %s (%s bindings for %s)\n", out, target, rest[0])
}
