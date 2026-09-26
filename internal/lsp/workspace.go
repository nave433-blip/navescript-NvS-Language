package lsp

// Workspace-wide intelligence: workspace/symbol, textDocument/references,
// and cross-file go-to-definition by following imports.
//
// Scope notes (honest):
//   - Only open documents are searched for workspace/symbol and
//     references. Cross-file definition additionally falls back to
//     reading the imported file from disk when it isn't open.
//   - References are name-based across files. NvS imports evaluate in
//     the caller's scope (no namespacing), so a top-level name usually
//     is the same binding everywhere; shadowing across files can
//     produce extra matches, which is documented, not hidden.
//   - Bare package imports (import "pkgname") are not followed; only
//     relative and absolute file paths are.

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/navescript/nvs/internal/ast"
	"github.com/navescript/nvs/internal/lexer"
)

// ---------------------------------------------------------------------------
// URI <-> path
// ---------------------------------------------------------------------------

func uriToPath(uri string) (string, bool) {
	if !strings.HasPrefix(uri, "file://") {
		return "", false
	}
	p, err := url.PathUnescape(strings.TrimPrefix(uri, "file://"))
	if err != nil {
		return "", false
	}
	return p, true
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// Escape each path segment but keep the slashes: "file:///tmp/a b.nvs".
	segs := strings.Split(abs, "/")
	for i, seg := range segs {
		segs[i] = url.PathEscape(seg)
	}
	return "file://" + strings.Join(segs, "/")
}

// docSource returns the source for uri: the open buffer when present,
// otherwise the file on disk.
func (s *server) docSource(uri string) (string, bool) {
	if src, ok := s.getDoc(uri); ok {
		return src, true
	}
	if p, ok := uriToPath(uri); ok {
		if data, err := os.ReadFile(p); err == nil {
			return string(data), true
		}
	}
	return "", false
}

// openURIs returns the URIs of all open documents, sorted for stable output.
func (s *server) openURIs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.docs))
	for u := range s.docs {
		out = append(out, u)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// workspace/symbol
// ---------------------------------------------------------------------------

type workspaceSymbol struct {
	Name     string   `json:"name"`
	Kind     int      `json:"kind"`
	Location location `json:"location"`
}

func (s *server) workspaceSymbols(query string) []workspaceSymbol {
	q := strings.ToLower(query)
	var out []workspaceSymbol
	for _, uri := range s.openURIs() {
		src, ok := s.getDoc(uri)
		if !ok {
			continue
		}
		for _, b := range topLevelBindings(parseDoc(src), src) {
			if q != "" && !strings.Contains(strings.ToLower(b.name), q) {
				continue
			}
			out = append(out, workspaceSymbol{
				Name:     b.name,
				Kind:     symbolKindFor(b.kind),
				Location: location{URI: uri, Range: *nameRange(b.tok)},
			})
		}
	}
	if out == nil {
		out = []workspaceSymbol{}
	}
	return out
}

// ---------------------------------------------------------------------------
// textDocument/references
// ---------------------------------------------------------------------------

// identOccurrences returns the ranges of every IDENT token with the
// given literal in src (1-based line/col converted to 0-based LSP).
func identOccurrences(src, name string) []lspRange {
	var out []lspRange
	l := lexer.New(src)
	for {
		tok := l.NextToken()
		if tok.Type == lexer.EOF {
			break
		}
		if tok.Type == lexer.IDENT && tok.Literal == name {
			out = append(out, *nameRange(tok))
		}
	}
	return out
}

func sameBinding(a, b *binding) bool {
	return a != nil && b != nil && a.name == b.name &&
		a.tok.Line == b.tok.Line && a.tok.Column == b.tok.Column
}

// references finds all uses of the binding under the cursor. In the
// current file, occurrences are filtered scope-aware (only those that
// resolve to the same binding); in other open files, matches are
// name-based (see the package-scope honesty note above).
func (s *server) references(uri string, pos position) []location {
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
	var out []location
	for _, u := range s.openURIs() {
		usrc, ok := s.getDoc(u)
		if !ok {
			continue
		}
		if u == uri {
			uprog := parseDoc(usrc)
			for _, r := range identOccurrences(usrc, b.name) {
				// r is 0-based; resolve needs 1-based line.
				if rb := resolve(uprog, usrc, b.name, r.Start.Line+1); sameBinding(rb, b) {
					out = append(out, location{URI: u, Range: r})
				}
			}
			continue
		}
		for _, r := range identOccurrences(usrc, b.name) {
			out = append(out, location{URI: u, Range: r})
		}
	}
	if out == nil {
		out = []location{}
	}
	return out
}

// ---------------------------------------------------------------------------
// Cross-file definition: follow imports.
// ---------------------------------------------------------------------------

// importURIs resolves the file paths imported by src (relative to the
// importing file's directory, mirroring the evaluator) into file URIs.
func (s *server) importURIs(uri, src string) []string {
	base, ok := uriToPath(uri)
	if !ok {
		return nil
	}
	dir := filepath.Dir(base)
	var out []string
	seen := map[string]bool{}
	for _, st := range parseDoc(src).Statements {
		is, ok := st.(*ast.ImportStatement)
		if !ok || is.Path == nil {
			continue
		}
		p := is.Path.Value
		if filepath.IsAbs(p) {
			// absolute path import
		} else if strings.HasPrefix(p, ".") {
			p = filepath.Join(dir, p)
		} else {
			continue // bare package spec: not followed (documented)
		}
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			continue
		}
		u := pathToURI(p)
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// definitionInFile returns the location of name's top-level binding in
// the given source, if any.
func definitionInFile(uri, src, name string) *location {
	for _, b := range topLevelBindings(parseDoc(src), src) {
		if b.name == name {
			return &location{URI: uri, Range: *nameRange(b.tok)}
		}
	}
	return nil
}

// definitionCrossFile follows imports from uri looking for a top-level
// binding named name. Open buffers win over disk.
func (s *server) definitionCrossFile(uri, src, name string) *location {
	for _, u := range s.importURIs(uri, src) {
		usrc, ok := s.docSource(u)
		if !ok {
			continue
		}
		if loc := definitionInFile(u, usrc, name); loc != nil {
			return loc
		}
	}
	return nil
}
