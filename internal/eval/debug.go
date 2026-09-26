package eval

import (
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/object"
)

// Wave 17: hooks that let tooling (the terminal debugger, the profiler)
// observe and control evaluation without forking the evaluator.
//
// Honest scope: these are cooperative, synchronous hooks into the
// tree-walking evaluator. They cost one nil-check per Eval call when
// unused. They do NOT work with the experimental bytecode VM (nvs bc),
// which is a separate execution engine.

// Debugger is implemented by tooling that wants per-statement control.
// All methods are called synchronously on the evaluating goroutine;
// BeforeStmt may block (e.g. run an interactive prompt) and evaluation
// resumes when it returns.
type Debugger interface {
	// BeforeStmt runs before a statement executes. file is the path of
	// the file being evaluated (eval.CurrentFile at the time) — never
	// empty for real runs; statements from imported files and the
	// prelude report their own file so tooling attributes lines
	// correctly.
	BeforeStmt(line int, file string, env *object.Environment)
	EnterCall(name string)
	LeaveCall()
}

// ActiveDebugger, when non-nil, receives statement/call events.
var ActiveDebugger Debugger

// StmtLine returns the source line of a statement node, if it is one.
// Expression nodes and the synthetic program root return ok=false so
// hooks only fire once per statement.
func StmtLine(node ast.Node) (line int, ok bool) {
	switch n := node.(type) {
	case *ast.ExpressionStatement:
		return n.Token.Line, true
	case *ast.LetStatement:
		return n.Token.Line, true
	case *ast.ReturnStatement:
		return n.Token.Line, true
	case *ast.PrintStatement:
		return n.Token.Line, true
	case *ast.WhileStatement:
		return n.Token.Line, true
	case *ast.ForStatement:
		return n.Token.Line, true
	case *ast.ForInStatement:
		return n.Token.Line, true
	case *ast.BreakStatement:
		return n.Token.Line, true
	case *ast.ContinueStatement:
		return n.Token.Line, true
	case *ast.IfExpression:
		return n.Token.Line, true
	case *ast.BlockStatement:
		return n.Token.Line, true
	case *ast.ImportStatement:
		return n.Token.Line, true
	case *ast.ClassStatement:
		return n.Token.Line, true
	case *ast.TryStatement:
		return n.Token.Line, true
	case *ast.ThrowStatement:
		return n.Token.Line, true
	case *ast.DeferStatement:
		return n.Token.Line, true
	case *ast.EnumStatement:
		return n.Token.Line, true
	case *ast.RecordStatement:
		return n.Token.Line, true
	default:
		return 0, false
	}
}

// PackageResolver, when non-nil, maps a bare package import spec
// (e.g. `import "myutils"` or `import "myutils/extra.nvs"`) to a
// concrete file path on disk. It is installed by the `nvs pkg`
// tooling; the evaluator itself knows nothing about caches or git.
var PackageResolver func(spec string) (filePath string, ok bool)

// IsBarePackageSpec reports whether an import path looks like a package
// reference rather than a file path: no leading ./ or ../, not absolute,
// and either extensionless or a pkg/subpath.nvs form.
func IsBarePackageSpec(path string) bool {
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasSuffix(path, ".ns") || strings.HasSuffix(path, ".nvs") {
		// "pkg/file.nvs" IS a package subpath; "file.nvs" is a file.
		return strings.Contains(path, "/")
	}
	return true
}
