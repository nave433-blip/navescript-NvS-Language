package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twoFileWorkspace opens a.nvs (defines helper) and b.nvs (imports it)
// as real temp files so import resolution works exactly like the
// evaluator's. It returns the client and both file:// URIs.
func twoFileWorkspace(t *testing.T) (*testClient, string, string) {
	t.Helper()
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.nvs")
	bPath := filepath.Join(dir, "b.nvs")
	aSrc := "/// Double a number.\nfn helper(x) {\n  return x * 2\n}\nprint helper(21)\n"
	bSrc := "import \"./a.nvs\"\nlet tag = \"b\"\nprint helper(1)\n"
	if err := os.WriteFile(aPath, []byte(aSrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(bSrc), 0644); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClient(t)
	initialize(t, c)
	aURI := "file://" + aPath
	bURI := "file://" + bPath
	didOpen(t, c, aURI, aSrc)
	c.nextNotification("textDocument/publishDiagnostics") // drain
	didOpen(t, c, bURI, bSrc)
	c.nextNotification("textDocument/publishDiagnostics") // drain
	return c, aURI, bURI
}

func TestLSPWorkspaceSymbol(t *testing.T) {
	c, aURI, bURI := twoFileWorkspace(t)

	id := c.request("workspace/symbol", map[string]any{"query": "help"})
	raw := c.nextReply(id)
	syms := resultOf[[]workspaceSymbol](t, raw)
	if len(syms) != 1 {
		t.Fatalf("want 1 symbol for query 'help', got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "helper" || syms[0].Location.URI != aURI {
		t.Fatalf("wrong symbol: %+v", syms[0])
	}

	// Empty query returns every top-level symbol in both files.
	id = c.request("workspace/symbol", map[string]any{"query": ""})
	raw = c.nextReply(id)
	syms = resultOf[[]workspaceSymbol](t, raw)
	names := map[string]bool{}
	uris := map[string]bool{}
	for _, s := range syms {
		names[s.Name] = true
		uris[s.Location.URI] = true
	}
	if !names["helper"] || !names["tag"] {
		t.Fatalf("empty query should find helper and tag, got %+v", syms)
	}
	if !uris[aURI] || !uris[bURI] {
		t.Fatalf("want symbols from both files, got uris %v", uris)
	}

	// No match -> empty list (not null).
	id = c.request("workspace/symbol", map[string]any{"query": "zzz_no_such_thing"})
	raw = c.nextReply(id)
	syms = resultOf[[]workspaceSymbol](t, raw)
	if len(syms) != 0 {
		t.Fatalf("want no symbols, got %+v", syms)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPCrossFileDefinition(t *testing.T) {
	c, aURI, bURI := twoFileWorkspace(t)

	// `helper` call in b.nvs, 0-based line 2: "print " is 6 chars.
	id := c.request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": bURI},
		"position":     map[string]any{"line": 2, "character": 7},
	})
	raw := c.nextReply(id)
	locs := resultOf[[]location](t, raw)
	if len(locs) != 1 {
		t.Fatalf("want 1 cross-file definition, got %+v", locs)
	}
	if locs[0].URI != aURI {
		t.Fatalf("want definition in a.nvs (%s), got %s", aURI, locs[0].URI)
	}
	if locs[0].Range.Start.Line != 1 {
		t.Fatalf("want definition on line 1 (0-based) of a.nvs, got %+v", locs[0].Range)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPCrossFileDefinitionFromDisk(t *testing.T) {
	// Same as above, but a.nvs is NOT open: the server must fall back
	// to reading it from disk.
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.nvs")
	bPath := filepath.Join(dir, "b.nvs")
	aSrc := "fn helper(x) {\n  return x * 2\n}\n"
	bSrc := "import \"./a.nvs\"\nlet tag = \"b\"\nprint helper(1)\n"
	if err := os.WriteFile(aPath, []byte(aSrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(bSrc), 0644); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClient(t)
	initialize(t, c)
	bURI := "file://" + bPath
	didOpen(t, c, bURI, bSrc)
	c.nextNotification("textDocument/publishDiagnostics") // drain

	id := c.request("textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": bURI},
		"position":     map[string]any{"line": 2, "character": 7},
	})
	raw := c.nextReply(id)
	locs := resultOf[[]location](t, raw)
	if len(locs) != 1 {
		t.Fatalf("want 1 disk-fallback definition, got %+v", locs)
	}
	if !strings.HasSuffix(locs[0].URI, "/a.nvs") {
		t.Fatalf("want definition in a.nvs, got %s", locs[0].URI)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}

func TestLSPReferences(t *testing.T) {
	c, aURI, bURI := twoFileWorkspace(t)

	// Cursor on the `helper` call in a.nvs (0-based line 4: doc comment
	// line + fn + return + print).
	id := c.request("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": aURI},
		"position":     map[string]any{"line": 4, "character": 7},
		"context":      map[string]any{"includeDeclaration": true},
	})
	raw := c.nextReply(id)
	locs := resultOf[[]location](t, raw)
	// a.nvs: definition (line 1) + call (line 3); b.nvs: call (line 2).
	if len(locs) != 3 {
		t.Fatalf("want 3 references, got %d: %+v", len(locs), locs)
	}
	byURI := map[string][]int{}
	for _, l := range locs {
		byURI[l.URI] = append(byURI[l.URI], l.Range.Start.Line)
	}
	if len(byURI[aURI]) != 2 || len(byURI[bURI]) != 1 {
		t.Fatalf("want 2 refs in a.nvs and 1 in b.nvs, got %+v", byURI)
	}

	c.request("shutdown", nil)
	c.notify("exit", nil)
}
