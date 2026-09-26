package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ---------------------------------------------------------------------------
// doc — doc-comment extractor (rustdoc idea, honestly small)
// ---------------------------------------------------------------------------
//
// Parses each file, finds top-level fn/class/record/interface/const/enum
// declarations, collects the contiguous doc-comment lines immediately
// preceding each declaration, and emits Markdown:
//
//	# hello.ns
//
//	## add
//	`fn add(a: int, b: int): int`
//
//	Adds two numbers.
//
//	## Dog
//	`class Dog extends Animal`
//
//	### speak
//	`fn speak(volume: int): string`
//
// Doc comments are `///` lines; when a declaration has no `///` lines, plain
// contiguous `//` lines are used instead (backward compatible with existing
// code). A blank line between the comment and the declaration stops the
// scan. Comment text is emitted verbatim — no Markdown processing.
//
// What it does NOT do (honest limits):
//   - no nested declarations (closures, inner fns are ignored)
//   - no cross-references, no type linking, no index page
//   - signatures are reconstructed from the AST (param names + types +
//     defaults + return annotation), not copied from source

// DocEntry is one documented declaration. Classes and interfaces carry
// their documented methods in Children.
type DocEntry struct {
	Kind      string // "fn", "class", "method", "record", "interface", "const", "enum"
	Name      string
	Signature string
	Doc       string
	Line      int
	Children  []DocEntry // methods (class/interface only)
}

// ExtractDocs parses src and returns doc entries for top-level
// declarations. The second return value carries parser errors (nil when
// parsing succeeded).
func ExtractDocs(file, src string) ([]DocEntry, []string) {
	p := parser.New(lexer.New(src))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, p.Errors()
	}
	srcLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var out []DocEntry
	for _, s := range prog.Statements {
		if e, ok := docEntryFor(s, srcLines); ok {
			out = append(out, e)
		}
	}
	_ = file
	return out, nil
}

// ExtractDocsFile reads the file at path and extracts its doc entries.
func ExtractDocsFile(path string) ([]DocEntry, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	entries, perr := ExtractDocs(path, string(data))
	return entries, perr, nil
}

// WriteDocsFile renders entries as Markdown and writes them to
// <outDir>/<base>.md where <base> is the source file's name without its
// extension. It creates outDir when needed and returns the written path.
func WriteDocsFile(outDir, srcPath string, entries []DocEntry) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	if base == "" {
		base = "index"
	}
	dest := filepath.Join(outDir, base+".md")
	if err := os.WriteFile(dest, []byte(RenderMarkdown(srcPath, entries)), 0o644); err != nil {
		return "", err
	}
	return dest, nil
}

// docEntryFor builds a DocEntry for one top-level statement, or false when
// the statement is not a documentable declaration.
func docEntryFor(s ast.Statement, srcLines []string) (DocEntry, bool) {
	// @decorator above a fn: document the fn, anchored at the decorator line.
	if ds, ok := s.(*ast.DecoratorStatement); ok {
		if ls, ok := ds.Function.(*ast.LetStatement); ok {
			if fl, ok := ls.Value.(*ast.FunctionLiteral); ok {
				e := fnEntry(ls.Name.Value, fl, lineOf(ds), srcLines)
				e.Kind = "fn"
				return e, true
			}
		}
		return DocEntry{}, false
	}
	switch n := s.(type) {
	case *ast.LetStatement:
		// Top-level `fn name(...)` desugars to let name = fn(...).
		fl, ok := n.Value.(*ast.FunctionLiteral)
		if !ok {
			return DocEntry{}, false
		}
		e := fnEntry(n.Name.Value, fl, lineOf(n), srcLines)
		e.Kind = "fn"
		return e, true
	case *ast.ConstStatement:
		sig := "const " + n.Name.Value
		if n.Value != nil {
			v := n.Value.String()
			if len(v) > 60 {
				v = v[:57] + "..."
			}
			sig += " = " + v
		}
		return DocEntry{Kind: "const", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	case *ast.ClassStatement:
		sig := "class " + n.Name.Value
		if n.Parent != nil {
			sig += " extends " + n.Parent.Value
		}
		e := DocEntry{Kind: "class", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}
		for _, m := range n.Methods {
			me := fnEntry(m.Name.Value, &ast.FunctionLiteral{
				Parameters: m.Parameters,
				ParamTypes: m.ParamTypes, ReturnType: m.ReturnType,
			}, m.Token.Line, srcLines)
			me.Kind = "method"
			e.Children = append(e.Children, me)
		}
		return e, true
	case *ast.RecordStatement:
		fields := []string{}
		for _, f := range n.Fields {
			fields = append(fields, f.Value)
		}
		sig := fmt.Sprintf("record %s(%s)", n.Name.Value, strings.Join(fields, ", "))
		return DocEntry{Kind: "record", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	case *ast.InterfaceDecl:
		e := DocEntry{Kind: "interface", Name: n.Name.Value,
			Signature: "interface " + n.Name.Value,
			Doc:      precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}
		for _, m := range n.Methods {
			me := fnEntry(m.Name.Value, &ast.FunctionLiteral{
				Parameters: m.ParamNames, ParamTypes: m.ParamTypes,
				ReturnType: m.ReturnType,
			}, m.Token.Line, srcLines)
			me.Kind = "method"
			e.Children = append(e.Children, me)
		}
		return e, true
	case *ast.EnumStatement:
		members := []string{}
		for _, m := range n.Members {
			members = append(members, m.Value)
		}
		sig := fmt.Sprintf("enum %s { %s }", n.Name.Value, strings.Join(members, ", "))
		return DocEntry{Kind: "enum", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	}
	return DocEntry{}, false
}

// fnEntry builds the doc entry for a named function or method literal,
// with typed parameters, defaults, and the return annotation.
func fnEntry(name string, fl *ast.FunctionLiteral, line int, srcLines []string) DocEntry {
	params := []string{}
	for i, p := range fl.Parameters {
		ps := p.Value
		if i < len(fl.ParamTypes) && fl.ParamTypes[i] != nil {
			ps += ": " + fl.ParamTypes[i].String()
		}
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			ps += " = " + fl.Defaults[i].String()
		}
		params = append(params, ps)
	}
	sig := fmt.Sprintf("fn %s(%s)", name, strings.Join(params, ", "))
	if fl.ReturnType != nil {
		sig += ": " + fl.ReturnType.String()
	}
	return DocEntry{Name: name, Signature: sig,
		Doc: precedingDoc(srcLines, line), Line: line}
}

// precedingDoc collects the contiguous comment lines immediately above the
// 1-based declLine. A blank line (or any non-comment line) stops the scan.
// `///` lines are preferred; when none are present, plain `//` lines are
// used. `////` is not a doc comment.
func precedingDoc(srcLines []string, declLine int) string {
	var lines []string
	for i := declLine - 2; i >= 0; i-- { // line above the decl, 0-based
		trimmed := strings.TrimSpace(srcLines[i])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		lines = append([]string{trimmed}, lines...)
	}
	var doc, fallback []string
	for _, l := range lines {
		if strings.HasPrefix(l, "///") && !strings.HasPrefix(l, "////") {
			doc = append(doc, strings.TrimPrefix(strings.TrimPrefix(l, "///"), " "))
		} else {
			fallback = append(fallback, strings.TrimPrefix(strings.TrimPrefix(l, "//"), " "))
		}
	}
	if len(doc) > 0 {
		return strings.Join(doc, "\n")
	}
	return strings.Join(fallback, "\n")
}

// RenderMarkdown renders entries as Markdown with a module-level `# <file>`
// heading. Methods render as `###` subsections under their class.
func RenderMarkdown(file string, entries []DocEntry) string {
	var b strings.Builder
	b.WriteString("# " + file + "\n")
	for _, e := range entries {
		b.WriteString("\n## " + e.Name + "\n")
		b.WriteString("`" + e.Signature + "`\n")
		if e.Doc != "" {
			b.WriteString("\n" + e.Doc + "\n")
		}
		for _, m := range e.Children {
			b.WriteString("\n### " + m.Name + "\n")
			b.WriteString("`" + m.Signature + "`\n")
			if m.Doc != "" {
				b.WriteString("\n" + m.Doc + "\n")
			}
		}
	}
	return b.String()
}
