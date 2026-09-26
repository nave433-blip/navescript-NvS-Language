// Wave 17: deeper quantum — multi-qubit gates, seeded simulation,
// shot sampling, and ASCII circuit diagrams.
//
// SIMULATED QUANTUM COMPUTING DISCLAIMER: everything in this file is a
// classical state-vector simulation running on local hardware. It never
// touches a quantum computer, QPU, or any quantum hardware. Gates are
// numpy-style matrix algebra over complex128 amplitudes; "measurement"
// is sampling from a classical probability distribution.
package quantum

import (
	"fmt"
	"math/cmplx"
	"math/rand"
	"strings"
)

// ---------------------------------------------------------------------------
// Multi-qubit gates: Toffoli, controlled phase, swap.
// ---------------------------------------------------------------------------

// Toffoli returns the 8x8 Toffoli (CCNOT) matrix on a 3-qubit space:
// flips the target bit (qubit 2, basis |c1 c2 t>) iff both control bits
// (qubits 0 and 1) are 1. Basis order is little-endian: |000>..|111> with
// qubit 0 as the least significant bit.
func Toffoli() [][]complex128 {
	u := make([][]complex128, 8)
	for i := range u {
		u[i] = make([]complex128, 8)
		u[i][i] = 1
	}
	// |110> <-> |111>: indices 6 and 7.
	u[6][6], u[6][7] = 0, 1
	u[7][7], u[7][6] = 0, 1
	return u
}

// ApplyToffoli applies the Toffoli gate to state s (little-endian qubit
// numbering: qubit i toggles with stride 2^i), flipping target t iff both
// controls c1 and c2 are 1. The state is not mutated; a new normalized
// state is returned.
func ApplyToffoli(s State, c1, c2, t int) (State, error) {
	if err := wave17CheckQubits(len(s), []int{c1, c2, t}); err != nil {
		return nil, err
	}
	if c1 == c2 || c1 == t || c2 == t {
		return nil, fmt.Errorf("toffoli: the two controls and the target must be distinct qubits")
	}
	tmask := 1 << uint(t)
	cmask := (1 << uint(c1)) | (1 << uint(c2))
	out := make(State, len(s))
	copy(out, s)
	for i := 0; i < len(out); i++ {
		if i&cmask == cmask {
			j := i ^ tmask
			if i < j {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return Normalize(out), nil
}

// ControlledPhase returns the 4x4 controlled-phase matrix on a 2-qubit
// space (control qubit 0, target qubit 1): it multiplies |11> by e^{i*phi}
// and leaves the other basis states alone.
func ControlledPhase(phi float64) [][]complex128 {
	u := make([][]complex128, 4)
	for i := range u {
		u[i] = make([]complex128, 4)
		u[i][i] = 1
	}
	u[3][3] = cmplx.Exp(complex(0, phi))
	return u
}

// ApplyControlledPhase applies a controlled-phase gate to state s:
// amplitudes where both control c and target t are 1 pick up e^{i*phi}.
func ApplyControlledPhase(s State, c, t int, phi float64) (State, error) {
	if err := wave17CheckQubits(len(s), []int{c, t}); err != nil {
		return nil, err
	}
	if c == t {
		return nil, fmt.Errorf("cphase: control and target must be different qubits")
	}
	factor := cmplx.Exp(complex(0, phi))
	out := make(State, len(s))
	copy(out, s)
	for i := range out {
		if i&(1<<uint(c)) != 0 && i&(1<<uint(t)) != 0 {
			out[i] *= factor
		}
	}
	return out, nil
}

// ApplySwap applies the SWAP gate to state s, exchanging the amplitudes
// of qubits a and b wherever they differ.
func ApplySwap(s State, a, b int) (State, error) {
	if err := wave17CheckQubits(len(s), []int{a, b}); err != nil {
		return nil, err
	}
	if a == b {
		return nil, fmt.Errorf("swap: the two qubits must differ")
	}
	mask := (1 << uint(a)) | (1 << uint(b))
	out := make(State, len(s))
	copy(out, s)
	for i := 0; i < len(out); i++ {
		ba := (i >> uint(a)) & 1
		bb := (i >> uint(b)) & 1
		if ba != bb {
			j := i ^ mask
			if i < j {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return Normalize(out), nil
}

// wave17CheckQubits validates qubit indices against a state vector length.
func wave17CheckQubits(dim int, qubits []int) error {
	n := 0
	for d := dim; d > 1; d >>= 1 {
		n++
	}
	if 1<<uint(n) != dim {
		return fmt.Errorf("state length %d is not a power of 2", dim)
	}
	for _, q := range qubits {
		if q < 0 || q >= n {
			return fmt.Errorf("qubit index %d out of range (state has %d qubits, indices 0..%d)", q, n, n-1)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Seeded simulation.
// ---------------------------------------------------------------------------

// Seeded reseeds the package-global math/rand source used by the
// package-level Measure function. Call it before Measure for reproducible
// demos. It is NOT safe for concurrent use.
func Seeded(seed int64) {
	rand.Seed(seed)
}

// Simulator is a measurement RNG-carrying simulator handle: the seeded,
// reproducible counterpart to the package-level functions. A Simulator
// created with NewSimulator(seed) always produces the same measurement
// outcomes for the same state and seed.
//
// Still a LOCAL CLASSICAL SIMULATION — no quantum hardware is involved.
type Simulator struct {
	rng *rand.Rand
}

// NewSimulator returns a Simulator seeded with seed.
func NewSimulator(seed int64) *Simulator {
	return &Simulator{rng: rand.New(rand.NewSource(seed))}
}

// Measure collapses state using the simulator's seeded RNG and returns the
// outcome index and the collapsed state. The input state is not mutated.
func (sm *Simulator) Measure(s State) (int, State) {
	p := Probabilities(s)
	r := sm.rng.Float64()
	var acc float64
	out := 0
	for i, pi := range p {
		acc += pi
		if r <= acc {
			out = i
			break
		}
	}
	ns := make(State, len(s))
	ns[out] = 1
	return out, ns
}

// Shots samples n measurement outcomes from state s WITHOUT collapsing
// or mutating it: each shot is drawn from an independent copy of s, so
// calling Shots leaves s (and any register wrapping it) untouched.
// Returns a histogram outcome -> count.
func Shots(s State, n int, rng *rand.Rand) map[int]int {
	hist := make(map[int]int)
	if n <= 0 || len(s) == 0 {
		return hist
	}
	p := Probabilities(s)
	// Cumulative distribution for inverse-transform sampling.
	cum := make([]float64, len(p))
	var acc float64
	for i, pi := range p {
		acc += pi
		cum[i] = acc
	}
	for k := 0; k < n; k++ {
		r := rng.Float64()
		out := len(p) - 1
		for i, c := range cum {
			if r <= c {
				out = i
				break
			}
		}
		hist[out]++
	}
	return hist
}

// ---------------------------------------------------------------------------
// ASCII circuit diagrams.
// ---------------------------------------------------------------------------

// Op describes one circuit operation for DrawCircuit.
type Op struct {
	Name     string  // gate name as shown on the wire, e.g. "H", "CNOT", "RX"
	Qubits   []int   // qubit indices; for controlled gates: control(s) first, target last
	Param    float64 // rotation angle, used when HasParam is true
	HasParam bool
}

// DrawCircuit renders an ASCII circuit diagram, one row per qubit.
//
// SIMULATED QUANTUM COMPUTING: this draws a plan for a local classical
// state-vector simulation, not hardware. Plain ASCII (-, |, +, [..]) is
// used so the diagram survives any terminal, log file, or copy/paste.
func DrawCircuit(n int, ops []Op) string {
	if n < 1 {
		return "(no qubits)\n"
	}
	// Column blocks: each op takes one block on each involved row; idle
	// rows get a matching empty block so columns line up.
	type block struct {
		label string // empty => plain wire
	}
	rows := make([][]string, n)
	label := func(op Op, q int) string {
		name := strings.ToUpper(op.Name)
		if op.HasParam {
			return fmt.Sprintf("%s(%.2f)", name, op.Param)
		}
		switch name {
		case "CNOT":
			if q == op.Qubits[0] {
				return "o"
			}
			return "X"
		case "CZ":
			return "o"
		case "SWAP":
			return "x"
		case "TOFFOLI":
			last := op.Qubits[len(op.Qubits)-1]
			if q == last {
				return "X"
			}
			return "o"
		case "CPHASE":
			return "o"
		case "MEASURE", "M":
			return "M"
		}
		return name
	}
	for _, op := range ops {
		involved := map[int]bool{}
		for _, q := range op.Qubits {
			if q >= 0 && q < n {
				involved[q] = true
			}
		}
		for q := 0; q < n; q++ {
			if involved[q] {
				rows[q] = append(rows[q], "["+label(op, q)+"]")
			} else {
				rows[q] = append(rows[q], "[--]")
			}
		}
	}
	var sb strings.Builder
	for q := 0; q < n; q++ {
		sb.WriteString(fmt.Sprintf("q%d: ", q))
		if len(rows[q]) == 0 {
			sb.WriteString("--")
		} else {
			sb.WriteString(strings.Join(rows[q], "--"))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
