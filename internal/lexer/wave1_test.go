package lexer

import "testing"

func TestWave1Tokens(t *testing.T) {
	input := `a?.b x |> f 1..10 1...5 [...a] a ? b : c a ?? b a?.5:b`

	tests := []struct {
		expectedType    TokenType
		expectedLiteral string
	}{
		{IDENT, "a"},
		{OPTIONAL_CHAIN, "?."},
		{IDENT, "b"},
		{IDENT, "x"},
		{PIPE, "|>"},
		{IDENT, "f"},
		{INT, "1"},
		{DOT, "."},
		{DOT, "."},
		{INT, "10"},
		{INT, "1"},
		{ELLIPSIS, "..."},
		{INT, "5"},
		{LBRACKET, "["},
		{ELLIPSIS, "..."},
		{IDENT, "a"},
		{RBRACKET, "]"},
		{IDENT, "a"},
		{QUESTION, "?"},
		{IDENT, "b"},
		{COLON, ":"},
		{IDENT, "c"},
		{IDENT, "a"},
		{NULL_COAL, "??"},
		{IDENT, "b"},
		// `?.` followed by a digit stays `?` `.` `5` (ternary-safe)
		{IDENT, "a"},
		{QUESTION, "?"},
		{DOT, "."},
		{INT, "5"},
		{COLON, ":"},
		{IDENT, "b"},
		{EOF, ""},
	}

	l := New(input)

	for i, tt := range tests {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - tokentype wrong. expected=%q, got=%q (literal=%q)",
				i, tt.expectedType, tok.Type, tok.Literal)
		}
		if tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}

func TestWave1InterpStringLexing(t *testing.T) {
	// The closing quote is suspended inside ${...} so nested strings work.
	input := `print "a ${"b"} c";`
	l := New(input)

	tok := l.NextToken() // print
	if tok.Type != PRINT {
		t.Fatalf("expected PRINT, got %q", tok.Type)
	}
	tok = l.NextToken() // string
	if tok.Type != STRING {
		t.Fatalf("expected STRING, got %q (%q)", tok.Type, tok.Literal)
	}
	if tok.Literal != `a ${"b"} c` {
		t.Fatalf("string literal wrong: %q", tok.Literal)
	}
}
