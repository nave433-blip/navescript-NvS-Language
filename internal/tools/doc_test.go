package tools

import (
	"strings"
	"testing"
)

func TestDocFn(t *testing.T) {
	src := "// Adds two numbers.\n// Second line.\nfn add(a, b) {\nreturn a + b\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Kind != "fn" || e.Name != "add" {
		t.Fatalf("bad kind/name: %+v", e)
	}
	if e.Signature != "fn add(a, b)" {
		t.Fatalf("bad signature: %q", e.Signature)
	}
	if e.Doc != "Adds two numbers.\nSecond line." {
		t.Fatalf("bad doc: %q", e.Doc)
	}
	if e.Line != 3 {
		t.Fatalf("bad line: %d", e.Line)
	}
}

func TestDocBlankLineStopsComment(t *testing.T) {
	src := "// Not a doc comment.\n\nfn f() {\nreturn 1\n}\n"
	entries, _ := ExtractDocs("t.ns", src)
	if len(entries) != 1 || entries[0].Doc != "" {
		t.Fatalf("blank line must stop doc collection: %+v", entries)
	}
}

func TestDocClassRecordInterface(t *testing.T) {
	src := "// A point.\nrecord Point(x, y)\n\n// A thing.\nclass Thing extends Base {\n}\n\n// A contract.\ninterface Shape {\n}\n\nfn undocumented() {\nreturn 1\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Kind != "record" || entries[0].Signature != "record Point(x, y)" {
		t.Fatalf("bad record entry: %+v", entries[0])
	}
	if entries[1].Kind != "class" || entries[1].Signature != "class Thing extends Base" {
		t.Fatalf("bad class entry: %+v", entries[1])
	}
	if entries[2].Kind != "interface" || entries[2].Signature != "interface Shape" {
		t.Fatalf("bad interface entry: %+v", entries[2])
	}
	if entries[3].Doc != "" {
		t.Fatalf("undocumented fn should have empty doc: %+v", entries[3])
	}
}

func TestDocFnDefaultsAndReturn(t *testing.T) {
	src := "fn greet(name = \"world\"): string {\nreturn \"hi \" + name\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry: %+v", entries)
	}
	if entries[0].Signature != `fn greet(name = "world"): string` {
		t.Fatalf("bad signature: %q", entries[0].Signature)
	}
}

func TestDocTopLevelOnly(t *testing.T) {
	src := "fn outer() {\n// inner doc\nfn inner() {\nreturn 1\n}\nreturn inner()\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 || entries[0].Name != "outer" {
		t.Fatalf("only top-level decls: %+v", entries)
	}
}

func TestDocNonDeclsIgnored(t *testing.T) {
	src := "// just a comment\nlet x = 1\n// another\nprint(x)\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 0 {
		t.Fatalf("plain lets are not documented: %+v", entries)
	}
}

func TestRenderMarkdown(t *testing.T) {
	entries := []DocEntry{
		{Kind: "fn", Name: "add", Signature: "fn add(a, b)", Doc: "Adds."},
		{Kind: "fn", Name: "sub", Signature: "fn sub(a, b)", Doc: ""},
	}
	md := RenderMarkdown("t.ns", entries, false)
	want := "## add\n`fn add(a, b)`\n\nAdds.\n\n## sub\n`fn sub(a, b)`\n"
	if md != want {
		t.Fatalf("got:\n%q\nwant:\n%q", md, want)
	}
	md2 := RenderMarkdown("t.ns", entries[:1], true)
	if !strings.HasPrefix(md2, "# t.ns\n\n") {
		t.Fatalf("multi-file should have file header: %q", md2)
	}
}

func TestDocParseError(t *testing.T) {
	_, perr := ExtractDocs("bad.ns", "fn = =\n")
	if len(perr) == 0 {
		t.Fatal("expected parser errors")
	}
}
