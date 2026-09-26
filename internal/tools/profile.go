package tools

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

// Wave 17: `nvs run --profile` — statement-execution profiler built on the
// eval.Debugger hook. Counts how many times each source line's statement
// executes, attributed per file (imports report their own file via the
// hook's file argument; the prelude is trusted runtime setup and never
// fires the hook). Tree-walker only; the experimental bytecode VM (nvs bc)
// is a separate engine without hooks.

type stmtProfiler struct {
	// file -> line -> hits
	counts map[string]map[int]int
}

func (p *stmtProfiler) BeforeStmt(line int, file string, env *object.Environment) {
	m := p.counts[file]
	if m == nil {
		m = map[int]int{}
		p.counts[file] = m
	}
	m[line]++
}
func (p *stmtProfiler) EnterCall(name string) {}
func (p *stmtProfiler) LeaveCall()            {}

// ProfileFile runs path under the statement profiler and writes the
// hottest lines to out. Returns an error for unreadable/unparseable
// files or runtime errors.
func ProfileFile(path string, top int, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading %s: %v", path, err)
	}
	l := lexer.New(string(data))
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return fmt.Errorf("parse errors in %s: %v", path, p.Errors())
	}
	prof := &stmtProfiler{counts: map[string]map[int]int{}}
	prev := eval.ActiveDebugger
	eval.ActiveDebugger = prof
	defer func() { eval.ActiveDebugger = prev }()

	eval.CurrentFile = path
	env := object.NewEnvironment()
	eval.LoadPrelude(env)
	result := eval.Eval(program, env)
	eval.DrainSpawnedTasks()
	if result != nil && result.Type() == object.ERROR_OBJ {
		return fmt.Errorf("runtime error: %s", result.Inspect())
	}

	// Cache source lines per file for display.
	sources := map[string][]string{}
	sourceOf := func(file string) []string {
		if ls, ok := sources[file]; ok {
			return ls
		}
		var ls []string
		if file == path || file == "" {
			ls = strings.Split(string(data), "\n")
		} else if d, err := os.ReadFile(file); err == nil {
			ls = strings.Split(string(d), "\n")
		}
		sources[file] = ls
		return ls
	}

	files := make([]string, 0, len(prof.counts))
	total := 0
	for f, m := range prof.counts {
		files = append(files, f)
		for _, c := range m {
			total += c
		}
	}
	sort.Strings(files)
	fmt.Fprintf(out, "profile: %d statement executions across %d file(s) (%s)\n", total, len(files), path)
	for _, f := range files {
		type entry struct {
			line  int
			count int
		}
		var entries []entry
		for line, count := range prof.counts[f] {
			entries = append(entries, entry{line, count})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })
		n := top
		if n <= 0 || n > len(entries) {
			n = len(entries)
		}
		display := f
		if display == "" {
			display = path
		}
		fmt.Fprintf(out, "--- %s ---\n", display)
		fmt.Fprintf(out, "%-8s %-8s %s\n", "LINE", "HITS", "SOURCE")
		lines := sourceOf(f)
		for _, e := range entries[:n] {
			src := ""
			if e.line >= 1 && e.line <= len(lines) {
				src = strings.TrimSpace(lines[e.line-1])
				if len(src) > 80 {
					src = src[:77] + "..."
				}
			}
			fmt.Fprintf(out, "%-8d %-8d %s\n", e.line, e.count, src)
		}
	}
	return nil
}
