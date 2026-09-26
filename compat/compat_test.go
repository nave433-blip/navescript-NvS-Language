// Package compat_test guards the frozen backwards-compatibility corpus.
//
// Every compat/*.nvs fixture exercises a 2.1.0-era language feature and must
// keep evaluating cleanly forever. The fixtures are FROZEN: they are never
// rewritten to accommodate a breaking change — if a fixture fails, the
// language regressed and the change must be reverted or migrated, never the
// fixture. See docs/COMPAT.md.
package compat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// TestCompatCorpus evaluates every frozen compat/*.nvs fixture and fails on
// any parse or eval error. Fixture output is not asserted — the corpus
// guards "it still runs", not exact formatting.
func TestCompatCorpus(t *testing.T) {
	files, err := filepath.Glob("*.nvs")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no compat fixtures found")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			env := object.NewEnvironment()
			eval.LoadPrelude(env)
			l := lexer.New(string(src))
			p := parser.New(l)
			prog := p.ParseProgram()
			if errs := p.Errors(); len(errs) > 0 {
				t.Fatalf("compat fixture %s no longer parses (language regressed?): %v", f, errs)
			}
			got := eval.Eval(prog, env)
			if errObj, ok := got.(*object.Error); ok {
				t.Fatalf("compat fixture %s no longer evaluates (language regressed?): %s", f, errObj.Message)
			}
		})
	}
}
