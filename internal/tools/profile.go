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
// executes, then reports the hottest lines. Tree-walker only; the
// experimental bytecode VM (nvs bc) is a separate engine without hooks.

type stmtProfiler struct {
	counts map[int]int
}

func (p *stmtProfiler) BeforeStmt(line int, env *object.Environment) {
	p.counts[line]++
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
	prof := &stmtProfiler{counts: map[int]int{}}
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

	lines := strings.Split(string(data), "\n")
	type entry struct {
		line  int
		count int
	}
	var entries []entry
	total := 0
	for line, count := range prof.counts {
		entries = append(entries, entry{line, count})
		total += count
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].count > entries[j].count })
	if top <= 0 || top > len(entries) {
		top = len(entries)
	}
	fmt.Fprintf(out, "profile: %d statement executions across %d lines (%s)\n", total, len(entries), path)
	fmt.Fprintf(out, "%-8s %-8s %s\n", "LINE", "HITS", "SOURCE")
	for _, e := range entries[:top] {
		src := ""
		if e.line >= 1 && e.line <= len(lines) {
			src = strings.TrimSpace(lines[e.line-1])
			if len(src) > 80 {
				src = src[:77] + "..."
			}
		}
		fmt.Fprintf(out, "%-8d %-8d %s\n", e.line, e.count, src)
	}
	return nil
}
