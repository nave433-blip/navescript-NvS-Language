package eval

// Tests for the 2.2–2.9 track ports: low-level quantum primitives,
// physics constants, and self_eval.

import (
	"math"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func port29Run(t *testing.T, src string) object.Object {
	t.Helper()
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(src)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	out := Eval(prog, env)
	if isError(out) {
		t.Fatalf("eval error: %s", out.Inspect())
	}
	return out
}

func TestPort29QZeroProbs(t *testing.T) {
	out := port29Run(t, `qprob(qzero(2))`)
	arr, ok := out.(*object.Array)
	if !ok || len(arr.Elements) != 4 {
		t.Fatalf("want 4-element array, got %s", out.Inspect())
	}
	if f := arr.Elements[0].(*object.Float).Value; f != 1.0 {
		t.Fatalf("qzero(2) prob[0] = %v, want 1", f)
	}
	for _, el := range arr.Elements[1:] {
		if f := el.(*object.Float).Value; f != 0.0 {
			t.Fatalf("qzero(2) prob[i] = %v, want 0", f)
		}
	}
}

func TestPort29HadamardSuperposition(t *testing.T) {
	out := port29Run(t, `qprob(qgate(qzero(1), "H"))`)
	arr := out.(*object.Array)
	for _, el := range arr.Elements {
		if f := el.(*object.Float).Value; math.Abs(f-0.5) > 1e-9 {
			t.Fatalf("H|0> prob = %v, want 0.5", f)
		}
	}
}

func TestPort29QubitAndTensor(t *testing.T) {
	out := port29Run(t, `qprob(qtensor(qubit(0), qubit(1)))`)
	arr := out.(*object.Array)
	if len(arr.Elements) != 4 {
		t.Fatalf("want 4 probs, got %d", len(arr.Elements))
	}
	// |0> tensor |1> = |01>, index 1
	if f := arr.Elements[1].(*object.Float).Value; f != 1.0 {
		t.Fatalf("qtensor prob[1] = %v, want 1", f)
	}
}

func TestPort29MeasureReturnsValidOutcome(t *testing.T) {
	out := port29Run(t, `qmeasure(qgate(qzero(1), "H"))["outcome"]`)
	n := out.(*object.Integer).Value
	if n != 0 && n != 1 {
		t.Fatalf("outcome = %d, want 0 or 1", n)
	}
}

func TestPort29QinnerAndNormalize(t *testing.T) {
	out := port29Run(t, `qinner(qubit(0), qubit(0))`)
	arr := out.(*object.Array)
	re := arr.Elements[0].(*object.Float).Value
	if math.Abs(re-1.0) > 1e-9 {
		t.Fatalf("<0|0> re = %v, want 1", re)
	}
	out = port29Run(t, `qprob(qnormalize(qgate(qzero(1), "X")))`)
	arr = out.(*object.Array)
	if f := arr.Elements[1].(*object.Float).Value; f != 1.0 {
		t.Fatalf("X|0> prob[1] = %v, want 1", f)
	}
}

func TestPort29QgateErrors(t *testing.T) {
	env := object.NewEnvironment()
	LoadPrelude(env)
	for _, src := range []string{
		`qgate(qzero(1), "NOPE")`,
		`qgate("not-a-state", "H")`,
		`qmeasure(42)`,
		`physics_const("nope")`,
	} {
		l := lexer.New(src)
		p := parser.New(l)
		out := Eval(p.ParseProgram(), env)
		if !isError(out) {
			t.Fatalf("%s: want error, got %s", src, out.Inspect())
		}
	}
}

func TestPort29PhysicsConsts(t *testing.T) {
	out := port29Run(t, `physics_const("hbar")`)
	if f := out.(*object.Float).Value; math.Abs(f-1.054571817e-34) > 1e-40 {
		t.Fatalf("hbar = %v", f)
	}
	out = port29Run(t, `pi`)
	if f := out.(*object.Float).Value; f != math.Pi {
		t.Fatalf("pi = %v", f)
	}
	out = port29Run(t, `c_light`)
	if f := out.(*object.Float).Value; f != 299792458.0 {
		t.Fatalf("c_light = %v", f)
	}
}

func TestPort29SelfEval(t *testing.T) {
	out := port29Run(t, `self_eval("6 * 7")`)
	if n := out.(*object.Integer).Value; n != 42 {
		t.Fatalf("self_eval = %v, want 42", n)
	}
	// self_eval sees the full language, not just a subset
	out = port29Run(t, `self_eval("[1,2,3] |> len")`)
	if n := out.(*object.Integer).Value; n != 3 {
		t.Fatalf("self_eval pipeline = %v, want 3", n)
	}
	// parse errors become error values, not panics
	env := object.NewEnvironment()
	LoadPrelude(env)
	l := lexer.New(`self_eval("(((")`)
	p := parser.New(l)
	out = Eval(p.ParseProgram(), env)
	if !isError(out) || !strings.Contains(out.Inspect(), "self_eval parse") {
		t.Fatalf("want self_eval parse error, got %s", out.Inspect())
	}
}

func TestPort29PowerOperator(t *testing.T) {
	cases := []struct{ src, want string }{
		{`2 ** 3`, "8"},
		{`2 ** 0`, "1"},
		{`2 ** -1`, "0"},          // 2.9 semantics: negative int exponent -> 0
		{`2 * 3 ** 2`, "18"},     // ** binds tighter than *
		{`(2 ** 3) ** 2`, "64"},  // explicit grouping
		{`2.0 ** 2.0`, "4"},
	}
	for _, c := range cases {
		out := port29Run(t, c.src)
		if out.Inspect() != c.want {
			t.Errorf("%s = %s, want %s", c.src, out.Inspect(), c.want)
		}
	}
	if f := port29Run(t, `2.0 ** 0.5`).(*object.Float).Value; math.Abs(f-1.4142135623730951) > 1e-12 {
		t.Errorf(`2.0 ** 0.5 = %v, want ~1.4142135623730951`, f)
	}
}
