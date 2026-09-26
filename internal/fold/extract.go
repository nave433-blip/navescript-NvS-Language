package fold

// nvs extract: run @nvs blocks embedded in foreign source files.
//
// Markers are whole-line, language-appropriate comments:
//
//	Python:      # @nvs            ...  # @endnvs
//	JS/TS:       // @nvs           ...  // @endnvs
//	C/Rust/Go:   // @nvs  (or /* @nvs */) ... // @endnvs (or /* @endnvs */)
//
// Every block in a file runs in ONE shared bridge.Session, so later blocks
// see bindings from earlier ones. Each block's result prints as a JSON
// envelope (the same shape `nvs bridge` uses). Unbalanced markers are a
// loud error naming the line numbers.

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/bridge"
)

// Block is one extracted @nvs region.
type Block struct {
	Code      string
	StartLine int // first code line (1-based)
	EndLine   int // last code line (1-based)
}

var openers = []string{"# @nvs", "// @nvs", "/* @nvs */", "<!-- @nvs -->"}
var closers = []string{"# @endnvs", "// @endnvs", "/* @endnvs */", "<!-- @endnvs -->"}

func isMarker(line string, set []string) bool {
	t := strings.TrimSpace(line)
	for _, m := range set {
		if t == m {
			return true
		}
	}
	return false
}

// ExtractBlocks scans src for @nvs blocks.
func ExtractBlocks(src string) ([]Block, error) {
	var blocks []Block
	var cur *Block
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		lineno := i + 1
		switch {
		case isMarker(line, openers):
			if cur != nil {
				return nil, fmt.Errorf("extract: nested @nvs block opened at line %d (previous block opened at line %d was not closed)",
					lineno, cur.StartLine-1)
			}
			cur = &Block{StartLine: lineno + 1}
		case isMarker(line, closers):
			if cur == nil {
				return nil, fmt.Errorf("extract: @endnvs at line %d without an open @nvs block", lineno)
			}
			cur.EndLine = lineno - 1
			blocks = append(blocks, *cur)
			cur = nil
		default:
			if cur != nil {
				cur.Code += line + "\n"
			}
		}
	}
	if cur != nil {
		return nil, fmt.Errorf("extract: @nvs block opened at line %d was never closed with @endnvs", cur.StartLine-1)
	}
	return blocks, nil
}

// ExtractFile extracts @nvs blocks from path and runs them in one shared
// session, printing "# --- block N (file:a-b) ---" headers and JSON result
// envelopes to out.
func ExtractFile(path string, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("extract: cannot read %s: %v", path, err)
	}
	blocks, err := ExtractBlocks(string(data))
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("extract: no @nvs blocks found in %s", path)
	}
	s := bridge.NewSession()
	for i, b := range blocks {
		fmt.Fprintf(out, "# --- block %d (%s:%d-%d) ---\n", i+1, path, b.StartLine, b.EndLine)
		obj, evalErr := s.Eval(b.Code)
		fmt.Fprintln(out, s.Envelope(obj, evalErr))
	}
	return nil
}
