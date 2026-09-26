package tools

import (
	"os"
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
	md := RenderMarkdown("t.ns", entries)
	want := "# t.ns\n\n## add\n`fn add(a, b)`\n\nAdds.\n\n## sub\n`fn sub(a, b)`\n"
	if md != want {
		t.Fatalf("got:\n%q\nwant:\n%q", md, want)
	}
}

func TestDocParseError(t *testing.T) {
	_, perr := ExtractDocs("bad.ns", "fn = =\n")
	if len(perr) == 0 {
		t.Fatal("expected parser errors")
	}
}

func TestDocSlashSlashSlashPreferred(t *testing.T) {
	src := "/// Doc line one.\n/// Doc line two.\n// regular comment\nfn f() {\nreturn 1\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry: %+v", entries)
	}
	if entries[0].Doc != "Doc line one.\nDoc line two." {
		t.Fatalf("/// should win over //: %q", entries[0].Doc)
	}
}

func TestDocSlashSlashFallback(t *testing.T) {
	// No /// lines: plain // comments still work (backward compatible).
	src := "// Adds two numbers.\nfn add(a, b) {\nreturn a + b\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 || entries[0].Doc != "Adds two numbers." {
		t.Fatalf("want // fallback: %+v", entries)
	}
}

func TestDocTypedSignature(t *testing.T) {
	src := "fn add(a: int, b: int): int {\nreturn a + b\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry: %+v", entries)
	}
	if entries[0].Signature != "fn add(a: int, b: int): int" {
		t.Fatalf("bad typed signature: %q", entries[0].Signature)
	}
}

func TestDocClassMethods(t *testing.T) {
	src := "/// A calculator.\nclass Calc {\n/// Adds.\nfn add(a: int, b: int): int {\nreturn a + b\n}\nfn hidden() {\nreturn 0\n}\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 || entries[0].Kind != "class" {
		t.Fatalf("expected 1 class entry: %+v", entries)
	}
	if entries[0].Doc != "A calculator." {
		t.Fatalf("bad class doc: %q", entries[0].Doc)
	}
	if len(entries[0].Children) != 2 {
		t.Fatalf("expected 2 methods: %+v", entries[0].Children)
	}
	m := entries[0].Children[0]
	if m.Kind != "method" || m.Name != "add" || m.Doc != "Adds." {
		t.Fatalf("bad method entry: %+v", m)
	}
	if m.Signature != "fn add(a: int, b: int): int" {
		t.Fatalf("bad method signature: %q", m.Signature)
	}
	md := RenderMarkdown("t.ns", entries)
	if !strings.Contains(md, "### add\n`fn add(a: int, b: int): int`") {
		t.Fatalf("methods should render as ### subsections:\n%s", md)
	}
}

func TestDocConstAndEnum(t *testing.T) {
	src := "/// Max retries.\nconst MaxRetries = 3\n\n/// Colors.\nenum Color { Red, Green, Blue }\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries: %+v", entries)
	}
	if entries[0].Kind != "const" || entries[0].Signature != "const MaxRetries = 3" {
		t.Fatalf("bad const entry: %+v", entries[0])
	}
	if entries[0].Doc != "Max retries." {
		t.Fatalf("bad const doc: %q", entries[0].Doc)
	}
	if entries[1].Kind != "enum" || entries[1].Signature != "enum Color { Red, Green, Blue }" {
		t.Fatalf("bad enum entry: %+v", entries[1])
	}
}

func TestDocInterfaceMethods(t *testing.T) {
	src := "interface Speaker {\n  speak(volume: int): string\n}\n"
	entries, perr := ExtractDocs("t.ns", src)
	if len(perr) > 0 {
		t.Fatalf("parse errors: %v", perr)
	}
	if len(entries) != 1 || entries[0].Kind != "interface" {
		t.Fatalf("expected 1 interface entry: %+v", entries)
	}
	if len(entries[0].Children) != 1 {
		t.Fatalf("expected 1 method: %+v", entries[0].Children)
	}
	if entries[0].Children[0].Signature != "fn speak(volume: int): string" {
		t.Fatalf("bad iface method signature: %q", entries[0].Children[0].Signature)
	}
}

func TestWriteDocsFile(t *testing.T) {
	dir := t.TempDir()
	entries := []DocEntry{{Kind: "fn", Name: "add", Signature: "fn add(a, b)", Doc: "Adds."}}
	dest, err := WriteDocsFile(dir, "/some/path/hello.ns", entries)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasSuffix(dest, "hello.md") {
		t.Fatalf("bad dest path: %q", dest)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.HasPrefix(string(data), "# /some/path/hello.ns\n") {
		t.Fatalf("bad markdown:\n%s", data)
	}
}
