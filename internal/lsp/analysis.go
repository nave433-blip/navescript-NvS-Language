package lsp

import (
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/eval"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ---------------------------------------------------------------------------
// Bindings: user-declared names with their declaration sites.
// ---------------------------------------------------------------------------

// binding is one user-declared name: fn/let/const/class/enum/record/
// interface or a function parameter.
type binding struct {
	kind string // "fn", "let", "const", "class", "enum", "record", "interface", "param"
	name string
	tok  lexer.Token // the name token (1-based Line/Column)
	sig  string
	doc  string
}

// parseDoc parses src, tolerating errors: a partial program is still
// useful for hover/completion/definition.
func parseDoc(src string) *ast.Program {
	p := parser.New(lexer.New(src))
	return p.ParseProgram()
}

// fnSig renders a user function's signature from its literal.
func fnSig(name string, fl *ast.FunctionLiteral) string {
	var sb strings.Builder
	sb.WriteString("fn " + name + "(")
	for i, p := range fl.Parameters {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(p.Value)
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != nil {
			sb.WriteString(": " + fl.ParamTypes[i].String())
		} else if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			sb.WriteString(" = " + fl.Defaults[i].String())
		}
	}
	sb.WriteString(")")
	if fl.ReturnType != nil {
		sb.WriteString(": " + fl.ReturnType.String())
	}
	return sb.String()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// declareStmt records the top-level binding(s) a single statement introduces.
func declareStmt(s ast.Statement, src string, out *[]*binding) {
	switch n := s.(type) {
	case *ast.LetStatement:
		if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
			*out = append(*out, &binding{kind: "fn", name: n.Name.Value, tok: n.Name.Token,
				sig: fnSig(n.Name.Value, fl), doc: docAbove(src, n.Token.Line)})
		} else {
			*out = append(*out, &binding{kind: "let", name: n.Name.Value, tok: n.Name.Token,
				sig: "let " + n.Name.Value + " = " + trunc(n.Value.String(), 60)})
		}
	case *ast.TypedLetStatement:
		*out = append(*out, &binding{kind: "let", name: n.Name.Value, tok: n.Name.Token,
			sig: "let " + n.Name.Value + ": " + n.Type.String() + " = " + trunc(n.Value.String(), 60)})
	case *ast.ConstStatement:
		*out = append(*out, &binding{kind: "const", name: n.Name.Value, tok: n.Name.Token,
			sig: "const " + n.Name.Value + " = " + trunc(n.Value.String(), 60)})
	case *ast.ClassStatement:
		sig := "class " + n.Name.Value
		if n.Parent != nil {
			sig += " extends " + n.Parent.Value
		}
		*out = append(*out, &binding{kind: "class", name: n.Name.Value, tok: n.Name.Token,
			sig: sig, doc: docAbove(src, n.Token.Line)})
	case *ast.EnumStatement:
		*out = append(*out, &binding{kind: "enum", name: n.Name.Value, tok: n.Name.Token,
			sig: "enum " + n.Name.Value})
	case *ast.RecordStatement:
		fields := make([]string, len(n.Fields))
		for i, f := range n.Fields {
			fields[i] = f.Value
		}
		*out = append(*out, &binding{kind: "record", name: n.Name.Value, tok: n.Name.Token,
			sig: "record " + n.Name.Value + "(" + strings.Join(fields, ", ") + ")"})
	case *ast.InterfaceDecl:
		*out = append(*out, &binding{kind: "interface", name: n.Name.Value, tok: n.Name.Token,
			sig: "interface " + n.Name.Value, doc: docAbove(src, n.Token.Line)})
	case *ast.DestructureLetStatement:
		for _, id := range patternIdents(n.Pattern) {
			*out = append(*out, &binding{kind: "let", name: id.Value, tok: id.Token,
				sig: "let " + id.Value + " (destructured)"})
		}
	case *ast.DecoratorStatement:
		declareStmt(n.Function, src, out)
	}
}

func patternIdents(pat ast.Expression) []*ast.Identifier {
	var out []*ast.Identifier
	switch p := pat.(type) {
	case *ast.ArrayPattern:
		out = append(out, p.Elements...)
		if p.Rest != nil {
			out = append(out, p.Rest)
		}
	case *ast.HashPattern:
		for _, e := range p.Entries {
			out = append(out, e.Value)
		}
	}
	return out
}

// docAbove collects consecutive `///` comment lines immediately above the
// given 1-based declaration line.
func docAbove(src string, declLine int) string {
	lines := strings.Split(src, "\n")
	var docs []string
	for i := declLine - 2; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "///") {
			break
		}
		docs = append([]string{strings.TrimSpace(strings.TrimPrefix(t, "///"))}, docs...)
	}
	return strings.Join(docs, "\n")
}

// topLevelBindings returns bindings for every top-level statement.
func topLevelBindings(prog *ast.Program, src string) []*binding {
	var out []*binding
	for _, s := range prog.Statements {
		declareStmt(s, src, &out)
	}
	return out
}

// ---------------------------------------------------------------------------
// Scope-aware name resolution (same file).
// ---------------------------------------------------------------------------

type scope struct {
	parent *scope
	names  map[string]*binding
}

func (s *scope) add(b *binding) {
	if s.names == nil {
		s.names = map[string]*binding{}
	}
	s.names[b.name] = b
}

func (s *scope) lookup(name string) *binding {
	for sc := s; sc != nil; sc = sc.parent {
		if b, ok := sc.names[name]; ok {
			return b
		}
	}
	return nil
}

// childBlock describes a nested statement list plus extra bindings that
// are in scope for it (function params, loop variables, catch ids).
type childBlock struct {
	stmts []ast.Statement
	extra []*binding
}

// childBlocks returns the nested statement lists of a statement.
func childBlocks(s ast.Statement) []childBlock {
	switch n := s.(type) {
	case *ast.BlockStatement:
		return []childBlock{{stmts: n.Statements}}
	case *ast.LetStatement:
		if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
			return []childBlock{{stmts: fl.Body.Statements, extra: paramBindings(fl)}}
		}
	case *ast.TypedLetStatement:
		if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
			return []childBlock{{stmts: fl.Body.Statements, extra: paramBindings(fl)}}
		}
	case *ast.ConstStatement:
		if fl, ok := n.Value.(*ast.FunctionLiteral); ok {
			return []childBlock{{stmts: fl.Body.Statements, extra: paramBindings(fl)}}
		}
	case *ast.WhileStatement:
		out := []childBlock{{stmts: n.Body.Statements}}
		if n.OrElse != nil {
			out = append(out, childBlock{stmts: n.OrElse.Statements})
		}
		return out
	case *ast.ForStatement:
		var extra []*binding
		if n.Init != nil {
			declareStmt(n.Init, "", &extra)
		}
		out := []childBlock{{stmts: n.Body.Statements, extra: extra}}
		if n.OrElse != nil {
			out = append(out, childBlock{stmts: n.OrElse.Statements, extra: extra})
		}
		return out
	case *ast.ForInStatement:
		extra := []*binding{{kind: "let", name: n.Name.Value, tok: n.Name.Token,
			sig: "let " + n.Name.Value + " (loop variable)"}}
		out := []childBlock{{stmts: n.Body.Statements, extra: extra}}
		if n.OrElse != nil {
			out = append(out, childBlock{stmts: n.OrElse.Statements, extra: extra})
		}
		return out
	case *ast.TryStatement:
		out := []childBlock{{stmts: n.Body.Statements}}
		if n.Catch != nil {
			var extra []*binding
			if n.CatchId != nil {
				extra = append(extra, &binding{kind: "let", name: n.CatchId.Value, tok: n.CatchId.Token,
					sig: "let " + n.CatchId.Value + " (catch binding)"})
			}
			out = append(out, childBlock{stmts: n.Catch.Statements, extra: extra})
		}
		if n.Finally != nil {
			out = append(out, childBlock{stmts: n.Finally.Statements})
		}
		return out
	case *ast.ClassStatement:
		var out []childBlock
		for _, m := range n.Methods {
			out = append(out, childBlock{stmts: m.Body.Statements, extra: methodParamBindings(m)})
		}
		return out
	case *ast.ExpressionStatement:
		switch e := n.Expression.(type) {
		case *ast.IfExpression:
			out := []childBlock{{stmts: e.Consequence.Statements}}
			if e.Alternative != nil {
				out = append(out, childBlock{stmts: e.Alternative.Statements})
			}
			return out
		case *ast.MatchExpression:
			var out []childBlock
			for _, arm := range e.Arms {
				out = append(out, childBlock{stmts: arm.Body.Statements})
			}
			if e.Default != nil {
				out = append(out, childBlock{stmts: e.Default.Statements})
			}
			return out
		case *ast.FunctionLiteral:
			return []childBlock{{stmts: e.Body.Statements, extra: paramBindings(e)}}
		}
	}
	return nil
}

func paramBindings(fl *ast.FunctionLiteral) []*binding {
	out := make([]*binding, 0, len(fl.Parameters))
	for i, p := range fl.Parameters {
		sig := "param " + p.Value
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != nil {
			sig += ": " + fl.ParamTypes[i].String()
		}
		out = append(out, &binding{kind: "param", name: p.Value, tok: p.Token, sig: sig})
	}
	return out
}

func methodParamBindings(m *ast.ClassMethod) []*binding {
	out := make([]*binding, 0, len(m.Parameters))
	for i, p := range m.Parameters {
		sig := "param " + p.Value
		if i < len(m.ParamTypes) && m.ParamTypes[i] != nil {
			sig += ": " + m.ParamTypes[i].String()
		}
		out = append(out, &binding{kind: "param", name: p.Value, tok: p.Token, sig: sig})
	}
	return out
}

func tokenOf(s ast.Statement) lexer.Token {
	switch n := s.(type) {
	case *ast.LetStatement:
		return n.Token
	case *ast.TypedLetStatement:
		return n.Token
	case *ast.ConstStatement:
		return n.Token
	case *ast.ReturnStatement:
		return n.Token
	case *ast.ExpressionStatement:
		return n.Token
	case *ast.BlockStatement:
		return n.Token
	case *ast.WhileStatement:
		return n.Token
	case *ast.ForStatement:
		return n.Token
	case *ast.ForInStatement:
		return n.Token
	case *ast.BreakStatement:
		return n.Token
	case *ast.ContinueStatement:
		return n.Token
	case *ast.ClassStatement:
		return n.Token
	case *ast.InterfaceDecl:
		return n.Token
	case *ast.EnumStatement:
		return n.Token
	case *ast.RecordStatement:
		return n.Token
	case *ast.ImportStatement:
		return n.Token
	case *ast.PrintStatement:
		return n.Token
	case *ast.TryStatement:
		return n.Token
	case *ast.ThrowStatement:
		return n.Token
	case *ast.YieldStatement:
		return n.Token
	case *ast.DeferStatement:
		return n.Token
	case *ast.DecoratorStatement:
		return n.Token
	case *ast.DestructureLetStatement:
		return n.Token
	}
	return lexer.Token{}
}

// scopeAt returns the innermost scope active at 1-based line L.
func scopeAt(stmts []ast.Statement, src string, L int, parent *scope) *scope {
	sc := &scope{parent: parent}
	var decls []*binding
	for _, s := range stmts {
		declareStmt(s, src, &decls)
	}
	for _, b := range decls {
		sc.add(b)
	}
	// Descend into the last statement starting at or before L.
	var target ast.Statement
	for _, s := range stmts {
		if tokenOf(s).Line <= L && tokenOf(s).Line > 0 {
			target = s
		} else {
			break
		}
	}
	if target == nil {
		return sc
	}
	for _, cb := range childBlocks(target) {
		// Only descend into the block that can contain L: the one whose
		// first statement starts at or before L (blocks are ordered).
		if len(cb.stmts) == 0 {
			continue
		}
		first := tokenOf(cb.stmts[0]).Line
		if first > 0 && first <= L {
			inner := &scope{parent: sc}
			for _, b := range cb.extra {
				inner.add(b)
			}
			return scopeAt(cb.stmts, src, L, inner)
		}
	}
	return sc
}

// resolve finds the binding for name visible at 1-based line L.
// Only declarations on or before L are considered.
func resolve(prog *ast.Program, src, name string, L int) *binding {
	sc := scopeAt(prog.Statements, src, L, nil)
	b := sc.lookup(name)
	if b == nil || b.tok.Line > L {
		return nil
	}
	return b
}

// ---------------------------------------------------------------------------
// Identifier under cursor.
// ---------------------------------------------------------------------------

// identAt returns the IDENT token covering 1-based (line, col), if any.
func identAt(src string, line, col int) (lexer.Token, bool) {
	l := lexer.New(src)
	for {
		tok := l.NextToken()
		if tok.Type == lexer.EOF {
			return lexer.Token{}, false
		}
		if tok.Type != lexer.IDENT {
			continue
		}
		if tok.Line == line && tok.Column <= col && col < tok.Column+len(tok.Literal) {
			return tok, true
		}
	}
}

var builtinSet = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range eval.BuiltinNames() {
		m[n] = true
	}
	return m
}()

// ---------------------------------------------------------------------------
// Hover
// ---------------------------------------------------------------------------

func (s *server) hover(uri string, pos position) *hoverResult {
	src, ok := s.getDoc(uri)
	if !ok {
		return nil
	}
	tok, found := identAt(src, pos.Line+1, pos.Character+1)
	if !found {
		return &hoverResult{Contents: markupContent{Kind: "markdown", Value: "no documentation"}}
	}
	prog := parseDoc(src)
	if b := resolve(prog, src, tok.Literal, tok.Line); b != nil {
		var sb strings.Builder
		if b.doc != "" {
			sb.WriteString(b.doc + "\n\n")
		}
		sb.WriteString("```nvs\n" + b.sig + "\n```")
		return &hoverResult{
			Contents: markupContent{Kind: "markdown", Value: sb.String()},
			Range:    nameRange(b.tok),
		}
	}
	if builtinSet[tok.Literal] {
		return &hoverResult{
			Contents: markupContent{Kind: "markdown", Value: "`" + tok.Literal + "` — builtin"},
		}
	}
	return &hoverResult{Contents: markupContent{Kind: "markdown", Value: "no documentation"}}
}

func nameRange(tok lexer.Token) *lspRange {
	return &lspRange{
		Start: position{Line: tok.Line - 1, Character: tok.Column - 1},
		End:   position{Line: tok.Line - 1, Character: tok.Column - 1 + len(tok.Literal)},
	}
}

// ---------------------------------------------------------------------------
// Completion
// ---------------------------------------------------------------------------

var keywords = []string{
	"fn", "let", "const", "if", "else", "for", "while", "return",
	"break", "continue", "class", "extends", "interface", "enum",
	"record", "match", "case", "default", "switch", "import",
	"in", "new", "this", "true", "false", "null", "try", "catch",
	"throw", "finally", "defer", "yield", "and", "or", "print",
}

func completionKindFor(kind string) int {
	switch kind {
	case "fn":
		return cmpFunction
	case "class":
		return cmpClass
	case "enum":
		return cmpEnum
	case "const":
		return cmpVariable
	default:
		return cmpVariable
	}
}

func (s *server) completion(uri string) completionList {
	items := []completionItem{}
	for _, n := range eval.BuiltinNames() {
		items = append(items, completionItem{Label: n, Kind: cmpFunction, Detail: "builtin"})
	}
	for _, kw := range keywords {
		items = append(items, completionItem{Label: kw, Kind: cmpKeyword, Detail: "keyword"})
	}
	if src, ok := s.getDoc(uri); ok {
		seen := map[string]bool{}
		for _, b := range topLevelBindings(parseDoc(src), src) {
			if seen[b.name] {
				continue
			}
			seen[b.name] = true
			items = append(items, completionItem{Label: b.name, Kind: completionKindFor(b.kind), Detail: "user " + b.kind})
		}
	}
	return completionList{IsIncomplete: false, Items: items}
}

// ---------------------------------------------------------------------------
// Definition (same file only)
// ---------------------------------------------------------------------------

func (s *server) definition(uri string, pos position) []location {
	src, ok := s.getDoc(uri)
	if !ok {
		return []location{}
	}
	tok, found := identAt(src, pos.Line+1, pos.Character+1)
	if !found {
		return []location{}
	}
	prog := parseDoc(src)
	b := resolve(prog, src, tok.Literal, tok.Line)
	if b == nil {
		return []location{}
	}
	return []location{{URI: uri, Range: *nameRange(b.tok)}}
}

// ---------------------------------------------------------------------------
// Document symbols (top level)
// ---------------------------------------------------------------------------

func symbolKindFor(kind string) int {
	switch kind {
	case "fn":
		return symFunction
	case "class":
		return symClass
	case "enum":
		return symEnum
	case "interface":
		return symInterface
	case "record":
		return symStruct
	case "const":
		return symConstant
	default:
		return symVariable
	}
}

func (s *server) documentSymbols(uri string) []documentSymbol {
	src, ok := s.getDoc(uri)
	if !ok {
		return []documentSymbol{}
	}
	prog := parseDoc(src)
	lines := strings.Split(src, "\n")
	var syms []documentSymbol
	for i, st := range prog.Statements {
		var bs []*binding
		declareStmt(st, src, &bs)
		for _, b := range bs {
			sel := nameRange(b.tok)
			endLine := len(lines) - 1
			if i+1 < len(prog.Statements) {
				if nl := tokenOf(prog.Statements[i+1]).Line - 2; nl >= sel.Start.Line {
					endLine = nl
				}
			}
			syms = append(syms, documentSymbol{
				Name: b.name,
				Kind: symbolKindFor(b.kind),
				Range: lspRange{
					Start: position{Line: sel.Start.Line, Character: 0},
					End:   position{Line: endLine, Character: 0},
				},
				SelectionRange: *sel,
			})
		}
	}
	return syms
}
