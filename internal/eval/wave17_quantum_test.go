// Wave 17f tests: deeper-quantum builtins (q_toffoli, q_cphase, q_seed,
// q_shots, circuit builder).
//
// SIMULATED QUANTUM COMPUTING DISCLAIMER: every assertion below runs on
// the LOCAL classical state-vector simulator — matrix algebra over
// complex128 amplitudes, never quantum hardware. Statistical assertions
// use seeded RNGs so they are deterministic, never flaky.
package eval

import (
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/object"
)

// wave17HistOf evals NvS code that must yield a hash of integer->integer
// (q_shots / circuit run output) and returns it as a Go map.
func wave17HistOf(t *testing.T, code string) map[int64]int64 {
	t.Helper()
	got := testEval(t, code)
	if isError(got) {
		t.Fatalf("eval %q: unexpected error: %s", code, got.Inspect())
	}
	h, ok := got.(*object.Hash)
	if !ok {
		t.Fatalf("eval %q: got %s, want hash", code, got.Type())
	}
	out := make(map[int64]int64, len(h.Pairs))
	for _, p := range h.Pairs {
		k, ok1 := p.Key.(*object.Integer)
		v, ok2 := p.Value.(*object.Integer)
		if !ok1 || !ok2 {
			t.Fatalf("eval %q: histogram holds non-integers: %s", code, got.Inspect())
		}
		out[k.Value] = v.Value
	}
	return out
}

func TestWave17ToffoliBuiltin(t *testing.T) {
	// |110> -> |111>; other control patterns leave the target alone.
	expectProbs(t, `let r = qalloc(3)
q_x(r, 0)
q_x(r, 1)
q_toffoli(r, 0, 1, 2)
q_probs(r)`, []float64{0, 0, 0, 0, 0, 0, 0, 1})
	expectProbs(t, `let r = qalloc(3)
q_x(r, 0)
q_toffoli(r, 0, 1, 2)
q_probs(r)`, []float64{0, 1, 0, 0, 0, 0, 0, 0})
	// Error paths.
	if got := testEval(t, `q_toffoli(qalloc(3), 0, 0, 1)`); !isError(got) {
		t.Fatalf("q_toffoli with duplicate controls should error, got %s", got.Inspect())
	}
	if got := testEval(t, `q_toffoli(qalloc(2), 0, 1, 5)`); !isError(got) {
		t.Fatalf("q_toffoli with out-of-range qubit should error, got %s", got.Inspect())
	}
}

func TestWave17CPhaseBuiltin(t *testing.T) {
	// Bell-ish (|00>+|11>)/sqrt2, then CPhase(pi/2) on (0,1):
	// |11> amplitude becomes i/sqrt2; probs unchanged.
	code := `let r = qalloc(2)
q_h(r, 0)
q_cnot(r, 0, 1)
q_cphase(r, 0, 1, 3.141592653589793 / 2)
q_state(r)`
	st := stateOf(t, code)
	if len(st) != 4 {
		t.Fatalf("got %d amplitudes, want 4", len(st))
	}
	// |11> is index 3: expect [~0, 0.7071].
	if abs(st[3][0]) > 1e-9 || abs(st[3][1]-0.7071067811865476) > 1e-9 {
		t.Fatalf("|11> amplitude = %v, want [0, 0.7071]", st[3])
	}
	// |00> untouched: [0.7071, 0].
	if abs(st[0][0]-0.7071067811865476) > 1e-9 || abs(st[0][1]) > 1e-9 {
		t.Fatalf("|00> amplitude = %v, want [0.7071, 0]", st[0])
	}
	if got := testEval(t, `q_cphase(qalloc(2), 1, 1, 0.5)`); !isError(got) {
		t.Fatalf("q_cphase with c==t should error, got %s", got.Inspect())
	}
}

func TestWave17SeedReproducible(t *testing.T) {
	// Same seed -> identical histograms, and q_shots does not collapse.
	h1 := wave17HistOf(t, `q_seed(2026)
let r = qalloc(2)
q_h(r, 0)
q_cnot(r, 0, 1)
q_shots(r, 500)`)
	h2 := wave17HistOf(t, `q_seed(2026)
let r = qalloc(2)
q_h(r, 0)
q_cnot(r, 0, 1)
q_shots(r, 500)`)
	if len(h1) != len(h2) {
		t.Fatalf("same seed gave different histogram shapes: %v vs %v", h1, h2)
	}
	for k, v := range h1 {
		if h2[k] != v {
			t.Fatalf("same seed diverged at outcome %d: %d vs %d", k, v, h2[k])
		}
	}
	// Only |00> (0) and |11> (3) can appear, roughly 50/50.
	if len(h1) != 2 || h1[0]+h1[3] != 500 {
		t.Fatalf("Bell shots histogram wrong: %v", h1)
	}
	if h1[0] < 200 || h1[0] > 300 {
		t.Fatalf("Bell shots outcome 0 = %d, want within 200..300", h1[0])
	}
}

func TestWave17ShotsNoCollapse(t *testing.T) {
	// q_shots must leave the register coherent: probs unchanged after.
	got := testEval(t, `let r = qalloc(2)
q_h(r, 0)
q_cnot(r, 0, 1)
let before = str(q_probs(r))
q_shots(r, 100)
let after = str(q_probs(r))
before == after`)
	if isError(got) {
		t.Fatalf("unexpected error: %s", got.Inspect())
	}
	if b, ok := got.(*object.Boolean); !ok || !b.Value {
		t.Fatalf("q_shots collapsed the register; probs changed: %s", got.Inspect())
	}
}

func TestWave17CircuitBuilderBell(t *testing.T) {
	// Bell state through the chainable builder: ~50/50 over seeded shots.
	h := wave17HistOf(t, `let cb = circuit(2)
cb["h"](0)
cb["cnot"](0, 1)
cb["run"](1000, 7)`)
	if len(h) != 2 || h[0]+h[3] != 1000 {
		t.Fatalf("builder Bell run wrong: %v", h)
	}
	if h[0] < 400 || h[0] > 600 {
		t.Fatalf("builder Bell outcome 0 = %d, want within 400..600", h[0])
	}
	// Chained single-expression form works too.
	got := testEval(t, `circuit(2)["h"](0)["x"](1)["probs"]()`)
	if isError(got) {
		t.Fatalf("chained builder call failed: %s", got.Inspect())
	}
	arr, ok := got.(*object.Array)
	if !ok || len(arr.Elements) != 4 {
		t.Fatalf("chained probs: got %s", got.Inspect())
	}
	// H(0), X(1): state |10>+|11> over sqrt2 -> probs [0,0,0.5,0.5].
	p2 := arr.Elements[2].(*object.Float).Value
	p3 := arr.Elements[3].(*object.Float).Value
	if abs(p2-0.5) > 1e-9 || abs(p3-0.5) > 1e-9 {
		t.Fatalf("chained probs = %v, want [0,0,0.5,0.5]", arr.Elements)
	}
	// draw() shows one row per qubit.
	got = testEval(t, `circuit(3)["h"](0)["toffoli"](0, 1, 2)["draw"]()`)
	if isError(got) {
		t.Fatalf("draw failed: %s", got.Inspect())
	}
	d := got.(*object.String).Value
	for _, row := range []string{"q0:", "q1:", "q2:"} {
		if !strings.Contains(d, row) {
			t.Fatalf("draw() missing row %q:\n%s", row, d)
		}
	}
	if !strings.Contains(d, "LOCAL SIMULATOR") {
		t.Fatalf("draw() missing the simulator disclaimer:\n%s", d)
	}
}

func TestWave17CircuitBuilderMethods(t *testing.T) {
	// rx/ry/rz/cphase/swap/cz/toffoli/measure paths on the builder.
	expectProbs(t, `circuit(1)["rx"](0, 3.141592653589793)["probs"]()`, []float64{0, 1})
	expectProbs(t, `circuit(1)["x"](0)["rz"](0, 3.141592653589793)["probs"]()`, []float64{0, 1})
	expectProbs(t, `let cb = circuit(2)
cb["x"](0)
cb["swap"](0, 1)
cb["probs"]()`, []float64{0, 0, 1, 0})
	// Seeded measure on a basis state is forced.
	got := testEval(t, `q_seed(5)
circuit(1)["x"](0)["measure"](0)`)
	if isError(got) {
		t.Fatalf("builder measure failed: %s", got.Inspect())
	}
	if i, ok := got.(*object.Integer); !ok || i.Value != 1 {
		t.Fatalf("measure of |1> = %s, want 1", got.Inspect())
	}
	// Error paths.
	if got := testEval(t, `circuit(2)["h"](9)`); !isError(got) {
		t.Fatalf("builder h(9) should error, got %s", got.Inspect())
	}
	if got := testEval(t, `circuit(2)["cnot"](1, 1)`); !isError(got) {
		t.Fatalf("builder cnot(1,1) should error, got %s", got.Inspect())
	}
	if got := testEval(t, `circuit(0)`); !isError(got) {
		t.Fatalf("circuit(0) should error, got %s", got.Inspect())
	}
}

func TestWave17GroverAmplifies(t *testing.T) {
	// 3-qubit Grover, marked |111>: two iterations must push the marked
	// state above 80% (theory: ~94.5%).
	got := testEval(t, `
fn oracle(r) {
  q_h(r, 2)
  q_toffoli(r, 0, 1, 2)
  q_h(r, 2)
}
fn diffuser(r) {
  q_h(r, 0)
  q_h(r, 1)
  q_h(r, 2)
  q_x(r, 0)
  q_x(r, 1)
  q_x(r, 2)
  q_h(r, 2)
  q_toffoli(r, 0, 1, 2)
  q_h(r, 2)
  q_x(r, 0)
  q_x(r, 1)
  q_x(r, 2)
  q_h(r, 0)
  q_h(r, 1)
  q_h(r, 2)
}
let r = qalloc(3)
q_h(r, 0)
q_h(r, 1)
q_h(r, 2)
oracle(r)
diffuser(r)
oracle(r)
diffuser(r)
q_probs(r)[7]`)
	if isError(got) {
		t.Fatalf("grover script failed: %s", got.Inspect())
	}
	p, ok := got.(*object.Float)
	if !ok {
		t.Fatalf("grover marked prob: got %s, want float", got.Type())
	}
	if p.Value < 0.8 {
		t.Fatalf("grover marked-state prob = %v, want > 0.8", p.Value)
	}
}

func TestWave17QSeedBuiltin(t *testing.T) {
	got := testEval(t, `q_seed(42)`)
	if isError(got) {
		t.Fatalf("q_seed failed: %s", got.Inspect())
	}
	if i, ok := got.(*object.Integer); !ok || i.Value != 42 {
		t.Fatalf("q_seed(42) = %s, want 42", got.Inspect())
	}
	if got := testEval(t, `q_seed("x")`); !isError(got) {
		t.Fatalf(`q_seed("x") should error, got %s`, got.Inspect())
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
