package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/navescript/nvs/internal/bridge"
	"github.com/navescript/nvs/internal/transpile"
)

// --- Wave 10: polyglot interop ---

func transpileHelp() {
	fmt.Print(`nvs transpile — honest-subset source transpiler (NvS -> JS/Python)

Usage:
  nvs transpile --to=js|python <file.ns>
  nvs transpile --help

Transpiles the documented transpilable subset of NvS (see LANGUAGE.md,
"Transpiler subset") to JavaScript or Python and prints the result to
stdout. A small runtime prelude of helpers (__div, __str, __truthy, ...)
is emitted with the code wherever the targets would otherwise differ
from NvS semantics (integer division, NvS truthiness, Inspect-based
string coercion, Go "percent-g" float formatting).

Anything outside the subset is a hard error naming the construct, e.g.:

  transpile error: unsupported construct: MatchExpression (pattern matching is outside the transpilable subset)

The transpiler never emits silently-wrong code. It assumes the input
program runs without errors in NvS; error behavior (const violations,
type errors, ...) is not replicated.
`)
}

func runTranspile(args []string) {
	target := ""
	var files []string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			transpileHelp()
			return
		case len(a) > 5 && a[:5] == "--to=":
			target = a[5:]
		case len(a) > 0 && a[0] == '-':
			fmt.Fprintf(os.Stderr, "transpile: unknown flag %s\n", a)
			os.Exit(1)
		default:
			files = append(files, a)
		}
	}
	if target == "" {
		fmt.Fprintln(os.Stderr, "transpile: --to=js|python is required")
		os.Exit(1)
	}
	if len(files) != 1 {
		fmt.Fprintln(os.Stderr, "usage: nvs transpile --to=js|python <file.ns>")
		os.Exit(1)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "transpile: %v\n", err)
		os.Exit(1)
	}
	out, err := transpile.TranspileSource(string(data), target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func bridgeHelp() {
	fmt.Print(`nvs bridge — JSON stdio bridge: drive NvS from any language

Usage:
  nvs bridge [--once]
  nvs bridge --help

Reads JSON requests (one per line) on stdin, writes JSON responses (one
per line) on stdout. One persistent interpreter for the whole session:
functions defined by one {"eval":...} are visible to later requests.
With --once, exactly one request line is processed and the bridge exits.

Protocol:
  {"eval": "<nvs source>", "id": <any>}          -> {"ok":true,"result":<json>,"id":<id>}
  {"call": "<name>", "args": [...], "id": <any>} -> {"ok":true,"result":<json>,"id":<id>}
  on any failure                                 -> {"ok":false,"error":"..."}
  malformed input line                           -> {"ok":false,"error":"invalid request: ..."}

"id" is optional and echoed verbatim when present so clients can match
replies to requests. Responses never contain silent nulls: a result value
outside the convertible set is an ok:false error naming the NvS type.

<json> covers NvS int, float, string, bool, null, array, hash. Any other
NvS value yields ok:false naming the type — never a silent mis-conversion.

Request limits: one JSON object per line, max 4 MiB per line. A line that
is not a JSON object, has trailing data, or names no known op is an
"invalid request" error — the session stays alive for the next line.
`)
}

func runBridge(args []string) {
	once := false
	for _, a := range args {
		switch a {
		case "-h", "--help":
			bridgeHelp()
			return
		case "--once":
			once = true
		default:
			fmt.Fprintf(os.Stderr, "bridge: unknown argument %s\n", a)
			os.Exit(1)
		}
	}
	sess := bridge.NewSession()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	for scanner.Scan() {
		fmt.Fprintln(out, sess.HandleLine(scanner.Text()))
		// Flush every response: the client is a persistent process
		// waiting on this line (flushing only at EOF would hang it).
		out.Flush()
		if once {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "bridge: reading stdin: %v\n", err)
		os.Exit(1)
	}
}
