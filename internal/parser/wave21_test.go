package parser

// Wave 21 tests: parse errors carry line:col positions and read like
// a human wrote them.

import (
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
)

func parseErr21(t *testing.T, src string) []string {
	t.Helper()
	p := New(lexer.New(src))
	p.ParseProgram()
	return p.Errors()
}

func TestWave21PeekErrorHasPosition(t *testing.T) {
	expectParseError(t, "let x = 1\nlet y = ;", "line 2:")
}

func TestWave21PeekErrorFriendly(t *testing.T) {
	// Missing expression after '=': should name what was expected/found.
	errs := parseErr21(t, "let x = ;")
	if len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
	msg := errs[0]
	if !strings.Contains(msg, "line 1:") || !strings.Contains(msg, "unexpected") {
		t.Fatalf("error not friendly: %q", msg)
	}
	t.Logf("message: %s", msg)
}

func TestWave21UnexpectedTokenHasPosition(t *testing.T) {
	errs := parseErr21(t, "let 123 = 4;")
	if len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
	msg := strings.Join(errs, "; ")
	if !strings.Contains(msg, "line 1:") {
		t.Fatalf("missing position: %q", msg)
	}
	t.Logf("message: %s", msg)
}

func TestWave21MultilinePosition(t *testing.T) {
	errs := parseErr21(t, "let a = 1;\nlet b = 2;\nlet c = ;")
	if len(errs) == 0 {
		t.Fatal("expected a parse error")
	}
	if !strings.Contains(errs[0], "line 3:") {
		t.Fatalf("wrong line in error: %q", errs[0])
	}
}
