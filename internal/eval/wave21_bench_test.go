package eval

import (
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func BenchmarkWave21IntArith(b *testing.B) {
	l := lexer.New(`1 + 2 + 3 + 4`)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		b.Fatalf("parse errors: %v", p.Errors())
	}
	env := object.NewEnvironment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(prog, env)
	}
}
