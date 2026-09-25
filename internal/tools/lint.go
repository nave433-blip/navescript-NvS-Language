package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ---------------------------------------------------------------------------
// lint — a small set of SOUND static checks (clippy idea, honest scope)
// ---------------------------------------------------------------------------
//
// Rules (documented in LANGUAGE.md; nothing deeper is claimed):
//
//	unused-binding  (warning) — a let/const binding whose name is never
//	                            referenced anywhere else in the file.
//	                            Excludes: function parameters (too noisy),
//	                            named fn declarations (entry points / library
//	                            APIs), loop variables, catch bindings, match
//	                            arm bindings. Any mention of the name —
//	                            including a plain assignment — counts as a use,
//	                            so this rule has no false positives on normal
//	                            code (it may miss shadowed cases: false
//	                            negatives, never false positives).
//	shadow-builtin    (warning) — let/const binding or plain assignment to a
//	                            name that is a Go-registered builtin
//	                            (len, print, …). Prelude (.ns) functions are
//	                            not covered; see eval.BuiltinNames.
//	unreachable-code  (warning) — statements in a block after a directly
//	                            preceding return/break/continue/throw.
//	                            Only direct block statements; no analysis
//	                            through if/while conditions (e.g. code after
//	                            `while (true) {}` is NOT flagged).
//	null-comparison   (style)   — `x == null` / `x != null`; suggests is_null().
//
// Exit code: 0 = clean, 1 = findings (or a file that fails to parse).

// Severity is the finding severity.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityStyle   Severity = "style"
)

// Rule names.
const (
	RuleUnusedBinding = "unused-binding"
	RuleShadowBuiltin = "shadow-builtin"
	RuleUnreachable   = "unreachable-code"
	RuleNullCompare   = "null-comparison"
	RuleParseError    = "parse-error"
)

// Finding is one lint result.
type Finding struct {
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s %s: %s", f.File, f.Line, f.Severity, f.Rule, f.Message)
}

// LintSource lints NvS source code. It returns findings and, separately,
// parser errors (a file that doesn't parse yields a parse-error finding and
// the raw parser error strings).
func LintSource(file, src string) ([]Finding, []string) {
	p := parser.New(lexer.New(src))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		f := []Finding{{
			File:     file,
			Line:     1,
			Rule:     RuleParseError,
			Severity: SeverityError,
			Message:  "could not parse file; no further checks ran",
		}}
		return f, p.Errors()
	}
	l := &linter{file: file, builtins: map[string]bool{}}
	for _, n := range eval.BuiltinNames() {
		l.builtins[n] = true
	}
	l.walkProgram(prog)
	l.finishUnused()
	sort.Slice(l.findings, func(i, j int) bool {
		if l.findings[i].Line != l.findings[j].Line {
			return l.findings[i].Line < l.findings[j].Line
		}
		return l.findings[i].Rule < l.findings[j].Rule
	})
	return l.findings, nil
}

// LintFile lints the file at path.
func LintFile(path string) ([]Finding, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	f, perr := LintSource(path, string(data))
	return f, perr, nil
}

// FindingsJSON renders findings as indented JSON.
func FindingsJSON(findings []Finding) string {
	if findings == nil {
		findings = []Finding{}
	}
	b, _ := json.MarshalIndent(findings, "", "  ")
	return string(b) + "\n"
}

type decl struct {
	name   string
	line   int
	isFunc bool // named fn / fn literal value: exempt from unused-binding
}

type linter struct {
	file     string
	builtins map[string]bool
	decls    []decl
	uses     map[string]int
	findings []Finding
}

func (l *linter) addFind(line int, rule string, sev Severity, msg string) {
	l.findings = append(l.findings, Finding{
		File: l.file, Line: line, Rule: rule, Severity: sev, Message: msg,
	})
}

func (l *linter) declare(name string, line int, isFunc bool) {
	l.decls = append(l.decls, decl{name: name, line: line, isFunc: isFunc})
	if l.builtins[name] {
		l.addFind(line, RuleShadowBuiltin, SeverityWarning,
			fmt.Sprintf("%q shadows the builtin of the same name; the builtin is unreachable past this point", name))
	}
}

func (l *linter) use(name string) {
	if l.uses == nil {
		l.uses = map[string]int{}
	}
	l.uses[name]++
}

func (l *linter) finishUnused() {
	for _, d := range l.decls {
		if d.isFunc {
			continue // named fns: entry points / library APIs
		}
		if l.uses[d.name] == 0 {
			l.addFind(d.line, RuleUnusedBinding, SeverityWarning,
				fmt.Sprintf("%q is declared but never used", d.name))
		}
	}
}

// --- walker ---

func (l *linter) walkProgram(p *ast.Program) {
	for _, s := range p.Statements {
		l.walkStmt(s)
	}
}

func (l *linter) walkBlock(b *ast.BlockStatement) {
	if b == nil {
		return
	}
	terminated := false
	for _, s := range b.Statements {
		if terminated {
			l.addFind(lineOf(s), RuleUnreachable, SeverityWarning,
				"unreachable code: a previous return/break/continue/throw always exits first")
		}
		switch s.(type) {
		case *ast.ReturnStatement, *ast.BreakStatement, *ast.ContinueStatement, *ast.ThrowStatement:
			terminated = true
		}
		l.walkStmt(s)
	}
}

func (l *linter) walkStmt(s ast.Statement) {
	switch n := s.(type) {
	case *ast.LetStatement:
		_, isFn := n.Value.(*ast.FunctionLiteral)
		l.declare(n.Name.Value, lineOf(n), isFn)
		l.walkExpr(n.Value)
	case *ast.ConstStatement:
		_, isFn := n.Value.(*ast.FunctionLiteral)
		l.declare(n.Name.Value, lineOf(n), isFn)
		l.walkExpr(n.Value)
	case *ast.TypedLetStatement:
		_, isFn := n.Value.(*ast.FunctionLiteral)
		l.declare(n.Name.Value, lineOf(n), isFn)
		l.walkExpr(n.Value)
	case *ast.DestructureLetStatement:
		for _, id := range patternBindings(n.Pattern) {
			l.declare(id.Value, lineOf(n), false)
		}
		l.walkExpr(n.Value)
	case *ast.ExpressionStatement:
		l.walkExpr(n.Expression)
	case *ast.ReturnStatement:
		if n.ReturnValue != nil {
			l.walkExpr(n.ReturnValue)
		}
	case *ast.ThrowStatement:
		l.walkExpr(n.Value)
	case *ast.YieldStatement:
		if n.Value != nil {
			l.walkExpr(n.Value)
		}
	case *ast.DeferStatement:
		l.walkExpr(n.Call)
	case *ast.PrintStatement:
		l.walkExpr(n.Value)
	case *ast.BlockStatement:
		l.walkBlock(n)
	case *ast.WhileStatement:
		l.walkExpr(n.Condition)
		l.walkBlock(n.Body)
		l.walkBlock(n.OrElse)
	case *ast.ForStatement:
		if n.Init != nil {
			l.walkStmt(n.Init)
		}
		if n.Condition != nil {
			l.walkExpr(n.Condition)
		}
		if n.Post != nil {
			l.walkExpr(n.Post)
		}
		l.walkBlock(n.Body)
		l.walkBlock(n.OrElse)
	case *ast.ForInStatement:
		// n.Name is a binding, not a use and not a let/const decl.
		l.walkExpr(n.Iterable)
		l.walkBlock(n.Body)
		l.walkBlock(n.OrElse)
	case *ast.BreakStatement, *ast.ContinueStatement:
	case *ast.ImportStatement:
	case *ast.ClassStatement:
		if n.Parent != nil {
			l.use(n.Parent.Value)
		}
		for _, m := range n.Methods {
			l.walkBlock(m.Body) // params excluded: too noisy
		}
	case *ast.InterfaceDecl:
		// Structural contract only; nothing executable to walk.
	case *ast.EnumStatement:
		// Members are bindings, not variable uses.
	case *ast.RecordStatement:
		// Fields are bindings.
	case *ast.TryStatement:
		l.walkBlock(n.Body)
		l.walkBlock(n.Catch) // CatchId is a binding, not a let/const decl
		l.walkBlock(n.Finally)
	case *ast.DecoratorStatement:
		l.use(n.Decorator.Value)
		l.walkStmt(n.Function)
	}
}

func (l *linter) walkExpr(e ast.Expression) {
	switch n := e.(type) {
	case nil:
		return
	case *ast.Identifier:
		l.use(n.Value)
	case *ast.IntegerLiteral, *ast.FloatLiteral, *ast.StringLiteral,
		*ast.Boolean, *ast.NullLiteral, *ast.ThisExpression:
	case *ast.PrefixExpression:
		l.walkExpr(n.Right)
	case *ast.InfixExpression:
		if (n.Operator == "==" || n.Operator == "!=") && isNullLit(n.Left, n.Right) {
			side := exprName(n.Left, n.Right)
			l.addFind(lineOf(n), RuleNullCompare, SeverityStyle,
				fmt.Sprintf("use is_null(%s) instead of %s %s null", side, side, n.Operator))
		}
		l.walkExpr(n.Left)
		l.walkExpr(n.Right)
	case *ast.IfExpression:
		l.walkExpr(n.Condition)
		l.walkBlock(n.Consequence)
		l.walkBlock(n.Alternative)
	case *ast.TernaryExpression:
		l.walkExpr(n.Condition)
		l.walkExpr(n.Consequence)
		l.walkExpr(n.Alternative)
	case *ast.FunctionLiteral:
		for _, d := range n.Defaults {
			l.walkExpr(d) // params themselves excluded: too noisy
		}
		l.walkBlock(n.Body)
	case *ast.CallExpression:
		l.walkExpr(n.Function)
		for _, a := range n.Arguments {
			l.walkExpr(a)
		}
	case *ast.NamedArgument:
		l.walkExpr(n.Value) // Name is the argument label, not a variable
	case *ast.ArrayLiteral:
		for _, el := range n.Elements {
			l.walkExpr(el)
		}
	case *ast.TupleLiteral:
		for _, el := range n.Elements {
			l.walkExpr(el)
		}
	case *ast.HashLiteral:
		for k, v := range n.Pairs {
			l.walkExpr(k)
			l.walkExpr(v)
		}
		for _, s := range n.Spreads {
			l.walkExpr(s)
		}
	case *ast.IndexExpression:
		l.walkExpr(n.Left)
		l.walkExpr(n.Index)
		if n.End != nil {
			l.walkExpr(n.End)
		}
	case *ast.AssignExpression:
		if l.builtins[n.Name.Value] {
			l.addFind(lineOf(n), RuleShadowBuiltin, SeverityWarning,
				fmt.Sprintf("assignment to %q shadows the builtin of the same name", n.Name.Value))
		}
		l.use(n.Name.Value) // a write still mentions the variable
		l.walkExpr(n.Value)
	case *ast.IndexAssignExpression:
		l.walkExpr(n.Left)
		l.walkExpr(n.Value)
	case *ast.MemberExpression:
		l.walkExpr(n.Object)
		// Property is a member name, not a variable reference: not a use.
	case *ast.MemberAssignExpression:
		l.walkExpr(n.Object)
		l.walkExpr(n.Value)
	case *ast.NewExpression:
		l.use(n.ClassName.Value)
		for _, a := range n.Arguments {
			l.walkExpr(a)
		}
	case *ast.MatchExpression:
		l.walkExpr(n.Value)
		for _, arm := range n.Arms {
			l.walkPattern(arm.Pattern)
			if arm.Guard != nil {
				l.walkExpr(arm.Guard)
			}
			l.walkBlock(arm.Body)
		}
		l.walkBlock(n.Default)
	case *ast.SpreadExpression:
		l.walkExpr(n.Value)
	case *ast.InterpolatedString:
		for _, part := range n.Parts {
			l.walkExpr(part)
		}
	case *ast.RangeExpression:
		l.walkExpr(n.Start)
		l.walkExpr(n.End)
	case *ast.OptionalChainExpression:
		l.walkExpr(n.Base)
		for _, link := range n.Links {
			switch link.Kind {
			case ast.ChainMember:
				// member name, not a variable reference
			case ast.ChainIndex:
				l.walkExpr(link.Index)
			case ast.ChainCall:
				for _, a := range link.Arguments {
					l.walkExpr(a)
				}
			}
		}
	}
}

// walkPattern walks a match-arm pattern. Bare identifiers in patterns BIND
// (wave 4), so they are neither declarations nor uses — except the head of a
// call pattern (e.g. Point in `case Point(x, y):`), which references a type.
func (l *linter) walkPattern(p ast.Expression) {
	switch n := p.(type) {
	case nil:
		return
	case *ast.Identifier:
		// binds; neither a use nor a let/const decl
	case *ast.ArrayPattern, *ast.HashPattern:
		// all bindings
	case *ast.CallExpression:
		l.walkExpr(n.Function)
		for _, a := range n.Arguments {
			l.walkPattern(a)
		}
	default:
		l.walkExpr(p) // literals etc.
	}
}

// patternBindings returns the identifiers bound by a let/const destructuring
// pattern.
func patternBindings(p ast.Expression) []*ast.Identifier {
	var out []*ast.Identifier
	switch n := p.(type) {
	case *ast.ArrayPattern:
		out = append(out, n.Elements...)
		if n.Rest != nil {
			out = append(out, n.Rest)
		}
	case *ast.HashPattern:
		for _, e := range n.Entries {
			out = append(out, e.Value)
		}
	}
	return out
}

func isNullLit(a, b ast.Expression) bool {
	_, ok1 := a.(*ast.NullLiteral)
	_, ok2 := b.(*ast.NullLiteral)
	return ok1 || ok2
}

// exprName returns a short display name for the non-null side of a
// null comparison, for the suggestion message.
func exprName(a, b ast.Expression) string {
	for _, e := range []ast.Expression{a, b} {
		if _, ok := e.(*ast.NullLiteral); !ok {
			if id, ok := e.(*ast.Identifier); ok {
				return id.Value
			}
			return "expr"
		}
	}
	return "expr"
}

// lineOf extracts the source line of a node via its token.
func lineOf(n ast.Node) int {
	switch t := n.(type) {
	case *ast.Program:
		return 1
	case *ast.LetStatement:
		return t.Token.Line
	case *ast.ConstStatement:
		return t.Token.Line
	case *ast.TypedLetStatement:
		return t.Token.Line
	case *ast.DestructureLetStatement:
		return t.Token.Line
	case *ast.ReturnStatement:
		return t.Token.Line
	case *ast.ExpressionStatement:
		return t.Token.Line
	case *ast.BlockStatement:
		return t.Token.Line
	case *ast.WhileStatement:
		return t.Token.Line
	case *ast.ForStatement:
		return t.Token.Line
	case *ast.ForInStatement:
		return t.Token.Line
	case *ast.BreakStatement:
		return t.Token.Line
	case *ast.ContinueStatement:
		return t.Token.Line
	case *ast.ImportStatement:
		return t.Token.Line
	case *ast.ClassStatement:
		return t.Token.Line
	case *ast.InterfaceDecl:
		return t.Token.Line
	case *ast.EnumStatement:
		return t.Token.Line
	case *ast.RecordStatement:
		return t.Token.Line
	case *ast.TryStatement:
		return t.Token.Line
	case *ast.ThrowStatement:
		return t.Token.Line
	case *ast.YieldStatement:
		return t.Token.Line
	case *ast.DecoratorStatement:
		return t.Token.Line
	case *ast.PrintStatement:
		return t.Token.Line
	case *ast.Identifier:
		return t.Token.Line
	case *ast.IntegerLiteral:
		return t.Token.Line
	case *ast.FloatLiteral:
		return t.Token.Line
	case *ast.StringLiteral:
		return t.Token.Line
	case *ast.InterpolatedString:
		return t.Token.Line
	case *ast.Boolean:
		return t.Token.Line
	case *ast.NullLiteral:
		return t.Token.Line
	case *ast.PrefixExpression:
		return t.Token.Line
	case *ast.InfixExpression:
		return t.Token.Line
	case *ast.IfExpression:
		return t.Token.Line
	case *ast.TernaryExpression:
		return t.Token.Line
	case *ast.FunctionLiteral:
		return t.Token.Line
	case *ast.CallExpression:
		return t.Token.Line
	case *ast.NamedArgument:
		return t.Token.Line
	case *ast.ArrayLiteral:
		return t.Token.Line
	case *ast.TupleLiteral:
		return t.Token.Line
	case *ast.HashLiteral:
		return t.Token.Line
	case *ast.IndexExpression:
		return t.Token.Line
	case *ast.AssignExpression:
		return t.Token.Line
	case *ast.IndexAssignExpression:
		return t.Token.Line
	case *ast.MemberExpression:
		return t.Token.Line
	case *ast.MemberAssignExpression:
		return t.Token.Line
	case *ast.NewExpression:
		return t.Token.Line
	case *ast.ThisExpression:
		return t.Token.Line
	case *ast.MatchExpression:
		return t.Token.Line
	case *ast.SpreadExpression:
		return t.Token.Line
	case *ast.RangeExpression:
		return t.Token.Line
	case *ast.OptionalChainExpression:
		return t.Token.Line
	default:
		return 0
	}
}
