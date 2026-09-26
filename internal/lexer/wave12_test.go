package lexer

import "testing"

// Wave 12: a leading shebang line is stripped so scripts can be
// chmod +x'd with #!/usr/bin/env nvs.
func TestShebangStripped(t *testing.T) {
	l := New("#!/usr/bin/env nvs\nlet x = 42;\n")
	tok := l.NextToken()
	if tok.Type != LET || tok.Literal != "let" {
		t.Fatalf("first token after shebang = %v (%q), want LET let", tok.Type, tok.Literal)
	}
}

func TestShebangOnlyInput(t *testing.T) {
	l := New("#!/usr/bin/env nvs")
	if tok := l.NextToken(); tok.Type != EOF {
		t.Fatalf("shebang-only input should lex to EOF, got %v", tok.Type)
	}
}

func TestNoShebangUnchanged(t *testing.T) {
	l := New("let x = 1;\n")
	if tok := l.NextToken(); tok.Type != LET {
		t.Fatalf("no-shebang input: first token = %v, want LET", tok.Type)
	}
}
