package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/parser"
)

// ---------------------------------------------------------------------------
// doc — MINIMAL doc-comment extractor (cargo-doc idea, honestly a stub)
// ---------------------------------------------------------------------------
//
// What it does: parses each file, finds TOP-LEVEL fn/class/record/interface
// declarations, collects the contiguous `//` comment lines immediately
// preceding each declaration, and emits Markdown:
//
//	## add
//	`fn add(a, b)`
//
//	Adds two numbers.
//
// What it does NOT do (by design — this is a stub, not rustdoc):
//   - no nested declarations (methods, closures, inner fns are ignored)
//   - no cross-references, no type linking, no index page
//   - no markdown processing of the comment text (emitted verbatim)
//   - signatures are reconstructed from the AST (param names + defaults +
//     return annotation when present), not copied from source

// DocEntry is one documented top-level declaration.
type DocEntry struct {
	Kind      string // "fn", "class", "record", "interface"
	Name      string
	Signature string
	Doc       string
	Line      int
}

// ExtractDocs parses src and returns doc entries for top-level
// fn/class/record/interface declarations. The second return value carries
// parser errors (nil when parsing succeeded).
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

// docEntryFor builds a DocEntry for one top-level statement, or false when
// the statement is not a documentable declaration.
func docEntryFor(s ast.Statement, srcLines []string) (DocEntry, bool) {
	// @decorator above a fn: document the fn, anchored at the decorator line.
	if ds, ok := s.(*ast.DecoratorStatement); ok {
		if ls, ok := ds.Function.(*ast.LetStatement); ok {
			if fl, ok := ls.Value.(*ast.FunctionLiteral); ok {
				return fnEntry(ls.Name.Value, fl, lineOf(ds), srcLines), true
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
		return fnEntry(n.Name.Value, fl, lineOf(n), srcLines), true
	case *ast.ClassStatement:
		sig := "class " + n.Name.Value
		if n.Parent != nil {
			sig += " extends " + n.Parent.Value
		}
		return DocEntry{Kind: "class", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	case *ast.RecordStatement:
		fields := []string{}
		for _, f := range n.Fields {
			fields = append(fields, f.Value)
		}
		sig := fmt.Sprintf("record %s(%s)", n.Name.Value, strings.Join(fields, ", "))
		return DocEntry{Kind: "record", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	case *ast.InterfaceDecl:
		sig := "interface " + n.Name.Value
		return DocEntry{Kind: "interface", Name: n.Name.Value, Signature: sig,
			Doc: precedingDoc(srcLines, lineOf(n)), Line: lineOf(n)}, true
	}
	return DocEntry{}, false
}

// fnEntry builds the doc entry for a named function literal.
func fnEntry(name string, fl *ast.FunctionLiteral, line int, srcLines []string) DocEntry {
	params := []string{}
	for i, p := range fl.Parameters {
		ps := p.Value
		if i < len(fl.Defaults) && fl.Defaults[i] != nil {
			ps += " = " + fl.Defaults[i].String()
		}
		params = append(params, ps)
	}
	sig := fmt.Sprintf("fn %s(%s)", name, strings.Join(params, ", "))
	if fl.ReturnType != nil {
		sig += ": " + fl.ReturnType.String()
	}
	return DocEntry{Kind: "fn", Name: name, Signature: sig,
		Doc: precedingDoc(srcLines, line), Line: line}
}

// precedingDoc collects contiguous `//` comment lines immediately above the
// 1-based declLine. A blank line (or any non-comment line) stops the scan.
func precedingDoc(srcLines []string, declLine int) string {
	var doc []string
	for i := declLine - 2; i >= 0; i-- { // line above the decl, 0-based
		trimmed := strings.TrimSpace(srcLines[i])
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		text := strings.TrimPrefix(trimmed, "//")
		text = strings.TrimPrefix(text, " ")
		doc = append([]string{text}, doc...)
	}
	return strings.Join(doc, "\n")
}

// RenderMarkdown renders entries as Markdown. When multiFile is true, each
// file's entries are preceded by a `# <file>` header.
func RenderMarkdown(file string, entries []DocEntry, multiFile bool) string {
	var b strings.Builder
	if multiFile {
		b.WriteString("# " + file + "\n\n")
	}
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("## " + e.Name + "\n")
		b.WriteString("`" + e.Signature + "`\n")
		if e.Doc != "" {
			b.WriteString("\n" + e.Doc + "\n")
		}
	}
	return b.String()
}
