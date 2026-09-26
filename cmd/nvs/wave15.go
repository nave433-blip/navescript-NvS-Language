package main

// Wave 15: code folding between languages + backwards compatibility.
//
//   - `nvs import --from=python|js <file>`: fold foreign source into NvS
//     (documented subset; loud errors outside it).
//   - `nvs extract <file>`: run @nvs blocks embedded in a foreign file.

import (
	"fmt"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/fold"
)

func importHelp() {
	fmt.Print(`nvs import — fold foreign code into NvS (honest-subset importer)

  nvs import --from=python <file.py>     Convert Python to NvS (stdout)
  nvs import --from=js <file.js>         Convert JavaScript to NvS (stdout)
  nvs import --help

Only the documented subset converts (see docs/FOLDING.md). Anything
outside it fails loudly with the construct name and line number —
never silently-wrong output:

  import error: unsupported python construct: class definition (line 12) — Python classes are outside the importable subset
`)
}

func extractHelp() {
	fmt.Print(`nvs extract — run NvS blocks embedded in a foreign source file

  nvs extract <file>

Block markers are language-appropriate comments:

  Python:   # @nvs        ...  # @endnvs
  JS:       // @nvs       ...  // @endnvs
  C/Rust/Go:// @nvs       ...  // @endnvs   (or /* @nvs */ ... /* @endnvs */)

Every block runs in ONE shared session: later blocks see bindings from
earlier ones. Each block's result prints as a JSON envelope:

  # --- block 1 (app.py:3-5) ---
  {"ok":true,"result":42}

Unbalanced markers are a loud error naming the line numbers.
`)
}

func runImport(args []string) {
	from := ""
	var files []string
	for _, a := range args {
		switch {
		case a == "--help" || a == "-h":
			importHelp()
			return
		case strings.HasPrefix(a, "--from="):
			from = strings.TrimPrefix(a, "--from=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "import: unknown flag %s\n", a)
			fmt.Fprintln(os.Stderr, "usage: nvs import --from=python|js <file>")
			os.Exit(1)
		default:
			files = append(files, a)
		}
	}
	if from == "" {
		fmt.Fprintln(os.Stderr, "import: --from=python|js is required")
		fmt.Fprintln(os.Stderr, "usage: nvs import --from=python|js <file>")
		os.Exit(1)
	}
	if len(files) != 1 {
		fmt.Fprintln(os.Stderr, "usage: nvs import --from=python|js <file>")
		os.Exit(1)
	}
	out, err := fold.ImportFile(files[0], from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	fmt.Print(out)
}

func runExtract(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: nvs extract <file>")
		os.Exit(1)
	}
	for _, a := range args {
		if a == "--help" || a == "-h" {
			extractHelp()
			return
		}
	}
	for _, f := range args {
		if err := fold.ExtractFile(f, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}
}
