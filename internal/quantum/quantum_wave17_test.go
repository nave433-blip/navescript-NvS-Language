// Wave 17f tests: Toffoli, controlled phase, seeded simulation, shots,
// ASCII circuit diagrams.
//
// SIMULATED QUANTUM COMPUTING DISCLAIMER: everything here is a classical
// state-vector simulation on local hardware — matrix algebra over
// complex128, never quantum hardware.
package quantum

import (
	"math"
	"math/cmplx"
	"math/rand"
	"strings"
	"testing"
)

// basisState returns |bits> for 3 qubits (little-endian: qubit 0 = LSB).
func basisState3(b0, b1, b2 int) State {
	s := NewZero(3)
	s[b0|b1<<1|b2<<2] = 1
	s[0] = 0
	return s
}

func TestToffoliTruthTable(t *testing.T) {
	// Target starts at 0; it must flip iff both controls are 1.
	for c1 := 0; c1 <= 1; c1++ {
		for c2 := 0; c2 <= 1; c2++ {
			s := basisState3(c1, c2, 0)
			out, err := ApplyToffoli(s, 0, 1, 2)
			if err != nil {
				t.Fatalf("ApplyToffoli(c1=%d,c2=%d): %v", c1, c2, err)
			}
			wantTarget := 0
			if c1 == 1 && c2 == 1 {
				wantTarget = 1
			}
			want := basisState3(c1, c2, wantTarget)
			for i := range want {
				if cmplx.Abs(out[i]-want[i]) > 1e-12 {
					t.Fatalf("ApplyToffoli(c1=%d,c2=%d): amp[%d]=%v, want %v", c1, c2, i, out[i], want[i])
				}
			}
		}
	}
	// Non-adjacent qubits: controls 0 and 2, target 1.
	s := NewZero(3)
	s[0b101] = 1 // q0=1, q1=0, q2=1
	s[0] = 0
	out, err := ApplyToffoli(s, 0, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cmplx.Abs(out[0b111]-1) > 1e-12 {
		t.Fatalf("non-adjacent toffoli: amp[7]=%v, want 1", out[0b111])
	}
	// Distinctness is enforced.
	if _, err := ApplyToffoli(NewZero(3), 0, 0, 1); err == nil {
		t.Fatal("ApplyToffoli with c1==c2 should error")
	}
	if _, err := ApplyToffoli(NewZero(2), 0, 1, 2); err == nil {
		t.Fatal("ApplyToffoli with out-of-range qubit should error")
	}
	// The matrix itself: identity except |110><->|111|.
	u := Toffoli()
	for i := 0; i < 8; i++ {
		for j := 0; j < 8; j++ {
			want := 0.0
			switch {
			case i == j && i != 6 && i != 7:
				want = 1
			case (i == 6 && j == 7) || (i == 7 && j == 6):
				want = 1
			}
			if cmplx.Abs(u[i][j]-complex(want, 0)) > 1e-12 {
				t.Fatalf("Toffoli()[%d][%d]=%v, want %v", i, j, u[i][j], want)
			}
		}
	}
}

func TestControlledPhase(t *testing.T) {
	phi := math.Pi / 2
	// |11> picks up e^{i phi} = i; others are untouched.
	s := NewZero(2)
	s[3] = 1
	s[0] = 0
	out, err := ApplyControlledPhase(s, 0, 1, phi)
	if err != nil {
		t.Fatal(err)
	}
	if cmplx.Abs(out[3]-complex(0, 1)) > 1e-12 {
		t.Fatalf("|11> phase: got %v, want i", out[3])
	}
	for _, idx := range []int{0b00, 0b01, 0b10} {
		s := NewZero(2)
		s[idx] = 1
		s[0] = 0
		if idx == 0 {
			s = NewZero(2)
		}
		out, err := ApplyControlledPhase(s, 0, 1, phi)
		if err != nil {
			t.Fatal(err)
		}
		if cmplx.Abs(out[idx]-1) > 1e-12 {
			t.Fatalf("|%02b> should be untouched, got %v", idx, out[idx])
		}
	}
	// Matrix check: diag(1,1,1,e^{i phi}).
	u := ControlledPhase(phi)
	if cmplx.Abs(u[3][3]-cmplx.Exp(complex(0, phi))) > 1e-12 {
		t.Fatalf("ControlledPhase matrix [3][3]=%v", u[3][3])
	}
	// c == t is rejected.
	if _, err := ApplyControlledPhase(NewZero(2), 0, 0, phi); err == nil {
		t.Fatal("ApplyControlledPhase with c==t should error")
	}
}

func TestApplySwap(t *testing.T) {
	s := NewZero(2)
	s[1] = 1 // |01>: qubit 0 = 1
	s[0] = 0
	out, err := ApplySwap(s, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cmplx.Abs(out[2]-1) > 1e-12 { // |10>
		t.Fatalf("swap: got %v, want |10>", out)
	}
	if _, err := ApplySwap(s, 1, 1); err == nil {
		t.Fatal("ApplySwap with a==b should error")
	}
}

func TestSeededMeasureReproducible(t *testing.T) {
	s := NewZero(1)
	var err error
	s, err = ApplyNamedGate(s, "h", 0)
	if err != nil {
		t.Fatal(err)
	}
	a := NewSimulator(99)
	b := NewSimulator(99)
	for i := 0; i < 20; i++ {
		oa, _ := a.Measure(s)
		ob, _ := b.Measure(s)
		if oa != ob {
			t.Fatalf("same seed diverged at shot %d: %d vs %d", i, oa, ob)
		}
	}
	c := NewSimulator(100)
	differs := false
	for i := 0; i < 20; i++ {
		oa, _ := a.Measure(s)
		oc, _ := c.Measure(s)
		if oa != oc {
			differs = true
			break
		}
	}
	if !differs {
		t.Fatal("different seeds produced identical 20-shot sequences (astronomically unlikely)")
	}
}

func TestShots(t *testing.T) {
	// (|00>+|11>)/sqrt2 on qubits 0,1: 50/50 between |00> and |11>.
	s2 := NewZero(2)
	s2[0] = complex(1/math.Sqrt2, 0)
	s2[3] = complex(1/math.Sqrt2, 0)
	before := append(State(nil), s2...)

	h1 := Shots(s2, 1000, rand.New(rand.NewSource(7)))
	h2 := Shots(s2, 1000, rand.New(rand.NewSource(7)))
	if len(h1) != len(h2) {
		t.Fatal("same seed gave different histogram shapes")
	}
	for k, v := range h1 {
		if h2[k] != v {
			t.Fatalf("same seed diverged: outcome %d: %d vs %d", k, v, h2[k])
		}
	}
	total := 0
	for _, v := range h1 {
		total += v
	}
	if total != 1000 {
		t.Fatalf("histogram sums to %d, want 1000", total)
	}
	if len(h1) != 2 {
		t.Fatalf("expected exactly outcomes 0 and 3, got %v", h1)
	}
	if h1[0] < 400 || h1[0] > 600 {
		t.Fatalf("outcome 0 count %d outside 400..600", h1[0])
	}
	// The input state must not be mutated or collapsed.
	for i := range s2 {
		if cmplx.Abs(s2[i]-before[i]) > 1e-15 {
			t.Fatalf("Shots mutated the state at %d", i)
		}
	}
	if Shots(s2, 0, rand.New(rand.NewSource(1))) == nil {
		t.Fatal("Shots(0) should return an empty non-nil map")
	}
}

func TestSeededPackageMeasure(t *testing.T) {
	s := NewZero(1)
	var err error
	s, err = ApplyNamedGate(s, "h", 0)
	if err != nil {
		t.Fatal(err)
	}
	Seeded(12345)
	o1, _ := Measure(s)
	Seeded(12345)
	o2, _ := Measure(s)
	if o1 != o2 {
		t.Fatalf("Seeded() did not make Measure reproducible: %d vs %d", o1, o2)
	}
}

func TestDrawCircuit(t *testing.T) {
	ops := []Op{
		{Name: "H", Qubits: []int{0}},
		{Name: "CNOT", Qubits: []int{0, 1}},
		{Name: "RX", Qubits: []int{2}, Param: 0.5, HasParam: true},
		{Name: "TOFFOLI", Qubits: []int{0, 1, 2}},
		{Name: "MEASURE", Qubits: []int{1}},
	}
	d := DrawCircuit(3, ops)
	for q := 0; q < 3; q++ {
		if !strings.Contains(d, "q"+string(rune('0'+q))+":") {
			t.Fatalf("diagram missing row for qubit %d:\n%s", q, d)
		}
	}
	for _, want := range []string{"H", "[o]", "[X]", "RX(0.50)", "[M]"} {
		if !strings.Contains(d, want) {
			t.Fatalf("diagram missing %q:\n%s", want, d)
		}
	}
	// One row per qubit.
	lines := strings.Split(strings.TrimSpace(d), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 rows, got %d:\n%s", len(lines), d)
	}
	if DrawCircuit(0, nil) == "" {
		t.Fatal("DrawCircuit(0) should return a placeholder, not empty")
	}
}
