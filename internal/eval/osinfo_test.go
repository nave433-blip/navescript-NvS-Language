package eval

import (
	"os"
	"runtime"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func osinfoEval(t *testing.T, src string) string {
	t.Helper()
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors for %q: %v", src, p.Errors())
	}
	got := Eval(prog, env)
	if isError(got) {
		t.Fatalf("eval error for %q: %s", src, got.Inspect())
	}
	s, ok := got.(*object.String)
	if !ok {
		t.Fatalf("got %s, want string", got.Type())
	}
	return s.Value
}

// TestOsBuiltins verifies the Wave 16 kernel-introspection builtins report
// the real runtime values.
func TestOsBuiltins(t *testing.T) {
	if got := osinfoEval(t, "os_name()"); got != runtime.GOOS {
		t.Errorf("os_name() = %q, want %q", got, runtime.GOOS)
	}
	wantKernel := "unix"
	if runtime.GOOS == "windows" {
		wantKernel = "nt"
	}
	if got := osinfoEval(t, "os_kernel()"); got != wantKernel {
		t.Errorf("os_kernel() = %q, want %q", got, wantKernel)
	}
	if got := osinfoEval(t, "os_sep()"); got != string(os.PathSeparator) {
		t.Errorf("os_sep() = %q, want %q", got, string(os.PathSeparator))
	}
	wantEOL := "\n"
	if runtime.GOOS == "windows" {
		wantEOL = "\r\n"
	}
	if got := osinfoEval(t, "os_eol()"); got != wantEOL {
		t.Errorf("os_eol() = %q, want %q", got, wantEOL)
	}
	if got := osinfoEval(t, "os_shell()"); got != shellName() {
		t.Errorf("os_shell() = %q, want %q", got, shellName())
	}
}
