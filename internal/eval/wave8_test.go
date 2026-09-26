package eval

// Wave 8 tests: quantum robbery — the LOCAL state-vector simulator.
//
// The loud disclaimer applies here too: nothing below touches quantum
// hardware. Gate matrices are checked against known states via q_probs
// (deterministic) and q_state; measurement randomness is only asserted
// where the outcome is forced (basis states), never via statistical
// sampling — flaky tests are worse than no tests.

import (
	"math"
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/object"
)

// probsOf evals NvS code that must yield an array of floats (q_probs)
// and returns the values.
func probsOf(t *testing.T, code string) []float64 {
	t.Helper()
	got := testEval(t, code)
	if isError(got) {
		t.Fatalf("eval %q: unexpected error: %s", code, got.Inspect())
	}
	arr, ok := got.(*object.Array)
	if !ok {
		t.Fatalf("eval %q: got %s, want array", code, got.Type())
	}
	out := make([]float64, len(arr.Elements))
	for i, e := range arr.Elements {
		f, ok := e.(*object.Float)
		if !ok {
			t.Fatalf("eval %q: element %d is %s, want float", code, i, e.Type())
		}
		out[i] = f.Value
	}
	return out
}

func expectProbs(t *testing.T, code string, want []float64) {
	t.Helper()
	got := probsOf(t, code)
	if len(got) != len(want) {
		t.Fatalf("eval %q: got %d probs, want %d", code, len(got), len(want))
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("eval %q: prob[%d] = %v, want %v (got %v)", code, i, got[i], want[i], got)
		}
	}
}

// stateOf evals NvS code that must yield q_state output: an array of
// [re, im] pairs.
func stateOf(t *testing.T, code string) [][2]float64 {
	t.Helper()
	got := testEval(t, code)
	if isError(got) {
		t.Fatalf("eval %q: unexpected error: %s", code, got.Inspect())
	}
	arr, ok := got.(*object.Array)
	if !ok {
		t.Fatalf("eval %q: got %s, want array", code, got.Type())
	}
	out := make([][2]float64, len(arr.Elements))
	for i, e := range arr.Elements {
		pair, ok := e.(*object.Array)
		if !ok || len(pair.Elements) != 2 {
			t.Fatalf("eval %q: element %d is not a [re, im] pair", code, i)
		}
		re, ok1 := pair.Elements[0].(*object.Float)
		im, ok2 := pair.Elements[1].(*object.Float)
		if !ok1 || !ok2 {
			t.Fatalf("eval %q: element %d pair holds non-floats", code, i)
		}
		out[i] = [2]float64{re.Value, im.Value}
	}
	return out
}

// ---- allocation ----

func TestWave8Alloc(t *testing.T) {
	expectInspect(t, `q_nqubits(qalloc(3))`, `3`)
	got := testEval(t, `qalloc(1)`)
	if isError(got) {
		t.Fatalf("qalloc(1): unexpected error: %s", got.Inspect())
	}
	if _, ok := got.(*QRegister); !ok {
		t.Fatalf("qalloc(1) returned %s, want *QRegister", got.Type())
	}
	if !strings.Contains(got.Inspect(), "LOCAL SIMULATOR") {
		t.Fatalf("QRegister Inspect() = %q, want the LOCAL SIMULATOR disclaimer", got.Inspect())
	}
}

func TestWave8AllocErrors(t *testing.T) {
	expectErrorContains(t, `qalloc(0)`, "at least 1 qubit")
	expectErrorContains(t, `qalloc(-2)`, "at least 1 qubit")
	expectErrorContains(t, `qalloc(25)`, "simulator limit: at most 24 qubits")
	expectErrorContains(t, `qalloc(1.5)`, "must be an integer")
	expectErrorContains(t, `qalloc("2")`, "must be an integer")
}

// ---- single-qubit gates on known states ----

func TestWave8HGate(t *testing.T) {
	expectProbs(t, `let q = qalloc(1); q_h(q, 0); q_probs(q)`, []float64{0.5, 0.5})
	// H is its own inverse: H(H|0>) = |0>.
	expectProbs(t, `let q = qalloc(1); q_h(q, 0); q_h(q, 0); q_probs(q)`, []float64{1, 0})
}

func TestWave8XGate(t *testing.T) {
	expectProbs(t, `let q = qalloc(1); q_x(q, 0); q_probs(q)`, []float64{0, 1})
	st := stateOf(t, `let q = qalloc(1); q_x(q, 0); q_state(q)`)
	if len(st) != 2 || math.Abs(st[0][0]) > 1e-12 || math.Abs(st[0][1]) > 1e-12 ||
		math.Abs(st[1][0]-1) > 1e-12 || math.Abs(st[1][1]) > 1e-12 {
		t.Fatalf("X|0> state = %v, want [[0 0] [1 0]]", st)
	}
}

func TestWave8YGate(t *testing.T) {
	// Y|0> = i|1>: probabilities [0, 1], amplitude [0, i].
	expectProbs(t, `let q = qalloc(1); q_y(q, 0); q_probs(q)`, []float64{0, 1})
	st := stateOf(t, `let q = qalloc(1); q_y(q, 0); q_state(q)`)
	if math.Abs(st[1][0]) > 1e-12 || math.Abs(st[1][1]-1) > 1e-12 {
		t.Fatalf("Y|0> amplitude[1] = %v, want [0 1] (i)", st[1])
	}
}

func TestWave8ZSTGates(t *testing.T) {
	// Z, S, T leave |0> alone (eigenstate, eigenvalue magnitude 1).
	expectProbs(t, `let q = qalloc(1); q_z(q, 0); q_probs(q)`, []float64{1, 0})
	expectProbs(t, `let q = qalloc(1); q_s(q, 0); q_probs(q)`, []float64{1, 0})
	expectProbs(t, `let q = qalloc(1); q_t(q, 0); q_probs(q)`, []float64{1, 0})
	// S|1> = i|1>: check the phase lands in the imaginary part.
	st := stateOf(t, `let q = qalloc(1); q_x(q, 0); q_s(q, 0); q_state(q)`)
	if math.Abs(st[1][0]) > 1e-12 || math.Abs(st[1][1]-1) > 1e-12 {
		t.Fatalf("S|1> amplitude[1] = %v, want [0 1] (i)", st[1])
	}
}

func TestWave8Rotations(t *testing.T) {
	// RX(pi)|0> = |1> (up to global phase).
	expectProbs(t, `let q = qalloc(1); q_rx(q, 0, 3.141592653589793); q_probs(q)`, []float64{0, 1})
	// RY(pi/2)|0> = (|0> + |1>)/sqrt(2).
	expectProbs(t, `let q = qalloc(1); q_ry(q, 0, 1.5707963267948966); q_probs(q)`, []float64{0.5, 0.5})
	// RZ is a pure phase on |0>: probabilities unchanged.
	expectProbs(t, `let q = qalloc(1); q_rz(q, 0, 1.234); q_probs(q)`, []float64{1, 0})
	// Integer angles are accepted too (radians).
	expectProbs(t, `let q = qalloc(1); q_rx(q, 0, 0); q_probs(q)`, []float64{1, 0})
}

// ---- two-qubit gates ----

func TestWave8BellState(t *testing.T) {
	// H on qubit 0 (LSB), then CNOT(0 -> 1): (|00> + |11>)/sqrt(2).
	expectProbs(t,
		`let q = qalloc(2); q_h(q, 0); q_cnot(q, 0, 1); q_probs(q)`,
		[]float64{0.5, 0, 0, 0.5})
}

func TestWave8CNOT(t *testing.T) {
	// X on control then CNOT(0 -> 1): |00> -> |10> -> |11>.
	expectProbs(t,
		`let q = qalloc(2); q_x(q, 0); q_cnot(q, 0, 1); q_probs(q)`,
		[]float64{0, 0, 0, 1})
	// CNOT with control |0> does nothing.
	expectProbs(t,
		`let q = qalloc(2); q_x(q, 1); q_cnot(q, 0, 1); q_probs(q)`,
		[]float64{0, 0, 1, 0})
}

func TestWave8CZ(t *testing.T) {
	// CZ(0,1) on |11> flips the phase of |11> to -|11>.
	st := stateOf(t, `let q = qalloc(2); q_x(q, 0); q_x(q, 1); q_cz(q, 0, 1); q_state(q)`)
	if math.Abs(st[3][0]+1) > 1e-12 || math.Abs(st[3][1]) > 1e-12 {
		t.Fatalf("CZ|11> amplitude[3] = %v, want [-1 0]", st[3])
	}
	expectProbs(t,
		`let q = qalloc(2); q_x(q, 0); q_x(q, 1); q_cz(q, 0, 1); q_probs(q)`,
		[]float64{0, 0, 0, 1})
}

func TestWave8Swap(t *testing.T) {
	// |01> (qubit0=1, little-endian index 1) swaps to |10> (index 2).
	expectProbs(t,
		`let q = qalloc(2); q_x(q, 0); q_swap(q, 0, 1); q_probs(q)`,
		[]float64{0, 0, 1, 0})
}

// ---- measurement (deterministic cases only — no flaky statistics) ----

func TestWave8MeasureBasisCollapses(t *testing.T) {
	// Measuring |1> always yields 1 and the state stays |1>.
	expectInspect(t, `let q = qalloc(1); q_x(q, 0); q_measure(q, 0)`, `1`)
	expectProbs(t, `let q = qalloc(1); q_x(q, 0); q_measure(q, 0); q_probs(q)`, []float64{0, 1})
	// Measuring |0> always yields 0.
	expectInspect(t, `let q = qalloc(1); q_measure(q, 0)`, `0`)
}

func TestWave8MeasureAllBasis(t *testing.T) {
	// Basis states measure deterministically; bits come back in qubit order.
	expectInspect(t,
		`let q = qalloc(3); q_x(q, 1); q_measure_all(q)`,
		`[0, 1, 0]`)
	expectProbs(t,
		`let q = qalloc(2); q_x(q, 0); q_x(q, 1); q_measure_all(q); q_probs(q)`,
		[]float64{0, 0, 0, 1})
}

func TestWave8MeasureCollapsesSuperposition(t *testing.T) {
	// After measuring one qubit of a Bell pair, the pair is a basis state:
	// exactly one probability is 1 and the rest are 0 — whichever outcome
	// the honest randomness chose.
	got := probsOf(t, `let q = qalloc(2); q_h(q, 0); q_cnot(q, 0, 1); q_measure(q, 0); q_probs(q)`)
	ones := 0
	for _, p := range got {
		if math.Abs(p-1) < 1e-9 {
			ones++
		} else if math.Abs(p) > 1e-9 {
			t.Fatalf("after collapse, probs = %v: not a basis state", got)
		}
	}
	if ones != 1 {
		t.Fatalf("after collapse, probs = %v: want exactly one 1", got)
	}
}

// ---- reset ----

func TestWave8Reset(t *testing.T) {
	expectProbs(t, `let q = qalloc(2); q_x(q, 0); q_x(q, 1); q_reset(q); q_probs(q)`,
		[]float64{1, 0, 0, 0})
}

// ---- circuit diagram ----

func TestWave8Circuit(t *testing.T) {
	got := testEval(t, `let q = qalloc(2); q_h(q, 0); q_cnot(q, 0, 1); q_circuit(q)`)
	if isError(got) {
		t.Fatalf("q_circuit: unexpected error: %s", got.Inspect())
	}
	s, ok := got.(*object.String)
	if !ok {
		t.Fatalf("q_circuit returned %s, want string", got.Type())
	}
	for _, want := range []string{"H", "●", "⊕", "LOCAL SIMULATOR", "q0:", "q1:", "CNOT(q0→q1)"} {
		if !strings.Contains(s.Value, want) {
			t.Fatalf("circuit diagram missing %q:\n%s", want, s.Value)
		}
	}
	// After reset the diagram is empty again.
	got2 := testEval(t, `let q = qalloc(1); q_h(q, 0); q_reset(q); q_circuit(q)`)
	s2 := got2.(*object.String)
	if !strings.Contains(s2.Value, "no gates applied yet") {
		t.Fatalf("circuit after reset should be empty, got:\n%s", s2.Value)
	}
}

// ---- backend registry: honest about hardware ----

func TestWave8Backend(t *testing.T) {
	expectInspect(t, `q_backend("local")`, `local-simulator`)
	expectErrorContains(t, `q_backend("azure")`, "not connected in this build")
	expectErrorContains(t, `q_backend("ibm")`, "not connected in this build")
	expectErrorContains(t, `q_backend(42)`, "must be a string")
}

// ---- error paths ----

func TestWave8BadQubitIndex(t *testing.T) {
	expectErrorContains(t, `let q = qalloc(2); q_h(q, 2)`, "out of range")
	expectErrorContains(t, `let q = qalloc(2); q_h(q, -1)`, "out of range")
	expectErrorContains(t, `let q = qalloc(2); q_measure(q, 7)`, "out of range")
	expectErrorContains(t, `let q = qalloc(2); q_cnot(q, 0, 5)`, "out of range")
	expectErrorContains(t, `let q = qalloc(2); q_h(q, "0")`, "must be an integer")
}

func TestWave8SameQubitTwoQubitGates(t *testing.T) {
	expectErrorContains(t, `let q = qalloc(2); q_cnot(q, 1, 1)`, "must be different")
	expectErrorContains(t, `let q = qalloc(2); q_swap(q, 0, 0)`, "must differ")
}

func TestWave8NotARegister(t *testing.T) {
	expectErrorContains(t, `q_h(42, 0)`, "must be a quantum register")
	expectErrorContains(t, `q_probs("nope")`, "must be a quantum register")
	expectErrorContains(t, `q_measure_all([1, 2])`, "must be a quantum register")
}

func TestWave8BadArgs(t *testing.T) {
	expectErrorContains(t, `let q = qalloc(1); q_rx(q, 0, "pi")`, "must be a number")
	expectErrorContains(t, `let q = qalloc(1); q_h(q)`, "want quantum register, qubit index")
	expectErrorContains(t, `qalloc()`, "want number of qubits")
}

// ---- misc integration ----

func TestWave8GatesReturnRegister(t *testing.T) {
	// Gates return the register so calls chain: H(H|0>) = |0>.
	expectProbs(t, `q_probs(q_h(q_h(qalloc(1), 0), 0))`, []float64{1, 0})
}

func TestWave8TypeOf(t *testing.T) {
	expectInspect(t, `type_of(qalloc(2))`, `qregister`)
}

func TestWave8LocalSimulatorImplementsBackend(t *testing.T) {
	// Compile-time assertion lives in wave8.go (var _ QuantumBackend);
	// here drive the backend directly for one honest end-to-end check.
	sim, err := newLocalSimulator(1)
	if err != nil {
		t.Fatalf("newLocalSimulator(1): %s", err)
	}
	var be QuantumBackend = sim
	if be.BackendName() != "local-simulator" {
		t.Fatalf("BackendName() = %q", be.BackendName())
	}
	if err := be.ApplyGate("X", []int{0}, nil); err != nil {
		t.Fatalf("ApplyGate(X): %s", err)
	}
	b, err := be.Measure(0)
	if err != nil {
		t.Fatalf("Measure: %s", err)
	}
	if b != 1 {
		t.Fatalf("Measure(|1>) = %d, want 1", b)
	}
	if err := be.ApplyGate("NOPE", []int{0}, nil); err == nil {
		t.Fatalf("ApplyGate(NOPE) should error")
	}
}
