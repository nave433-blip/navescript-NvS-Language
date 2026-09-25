package eval

// Wave 8 — quantum robbery: a real LOCAL state-vector quantum simulator.
//
// ============================================================================
// !!! THIS IS A LOCAL SIMULATOR, NOT QUANTUM HARDWARE. !!!
// ============================================================================
// Everything in this file runs on your own CPU. It simulates what an ideal,
// noise-free quantum computer WOULD do by tracking the full state vector
// (2^n complex amplitudes for n qubits) and applying real gate matrices to
// it — the same approach Qiskit Aer and Cirq's simulator take. Nothing here
// talks to a quantum processor, runs on qubits, or connects to Azure Quantum
// / IBM Quantum / any cloud backend. Gate results are computed, not
// measured from nature; the only genuine randomness is classical
// (crypto/rand) used when a measurement collapses the simulated state.
//
// The QuantumBackend interface is the forward-looking part: a future
// hardware backend (Azure Quantum, etc.) would implement this interface.
// Today the only implementation is the local simulator, and q_backend()
// says so honestly.
//
// Conventions (documented once, used everywhere):
//   - LITTLE-ENDIAN qubit numbering: qubit 0 is the LEAST significant bit.
//     State-vector index i corresponds to |q_{n-1} ... q_1 q_0> where bit j
//     of i is the value of qubit j. q_probs(q)[3] on 2 qubits is the
//     probability of |11> (qubit0=1, qubit1=1), q_probs(q)[2] is |10>
//     (qubit1=1, qubit0=0).
//   - Angles (q_rx/q_ry/q_rz) are in RADIANS.
//   - q_state returns [re, im] float pairs because NvS has no complex type.
//   - q_measure_all returns an ARRAY of bits in qubit order: element 0 is
//     qubit 0, element n-1 is qubit n-1.
//
// Simulator limits (honest): at most 24 qubits — 2^24 complex128 amplitudes
// is 256MB of state, and every gate application touches all of it (O(2^n)).
// A 25-qubit register would need 512MB and is refused with a loud error.

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"strings"

	"github.com/navescript/nvs/internal/object"
)

// maxSimQubits is the hard simulator cap: 2^24 complex128 = 256MB of state.
const maxSimQubits = 24

// ---------------------------------------------------------------------------
// QuantumBackend — the forward-looking abstraction.
// ---------------------------------------------------------------------------

// QuantumBackend is what NvS quantum registers execute against. The local
// state-vector simulator implements it today. A future Azure Quantum (or
// other hardware) backend would implement this same interface — q_backend()
// is the honest registry: it returns an error for any backend this build
// cannot actually reach instead of faking a connection.
type QuantumBackend interface {
	// BackendName identifies the backend, e.g. "local-simulator".
	BackendName() string
	// NumQubits is the register width this backend instance was created with.
	NumQubits() int
	// ApplyGate applies a named gate. Single-qubit gates take one target
	// and, for rotations, one parameter (radians). Two-qubit gates take two
	// targets (meaning depends on the gate: control/target for CNOT/CZ,
	// the pair for SWAP).
	ApplyGate(name string, targets []int, params []float64) error
	// Measure measures one qubit, collapsing the state. Returns 0 or 1.
	Measure(qubit int) (int, error)
	// MeasureAll measures every qubit (0..n-1 in order) and returns the
	// outcomes as bits in qubit order.
	MeasureAll() ([]int, error)
	// Probabilities returns 2^n outcome probabilities in little-endian
	// index order (index i = bit pattern with qubit 0 as LSB).
	Probabilities() []float64
	// Amplitudes returns a copy of the full state vector, same ordering.
	Amplitudes() []complex128
	// Reset returns the register to |0...0> and clears the recorded circuit.
	Reset()
	// CircuitDiagram returns an ASCII diagram of the gates applied so far.
	CircuitDiagram() string
}

// ---------------------------------------------------------------------------
// QRegister — the NvS object wrapping a backend.
// ---------------------------------------------------------------------------

// QRegister is the NvS-level quantum register object (created by qalloc).
// It is a mutable handle and passes by reference (like wave-6 channels and
// tasks: deepCopyMessage leaves it alone). It is NOT safe for concurrent
// use from multiple tasks — sharing a register across tasks is a data race
// and a bug in your program, per the wave-6 rules.
type QRegister struct {
	backend QuantumBackend
}

// qRegisterObjType is the object type tag for quantum registers.
const qRegisterObjType object.ObjectType = "QREGISTER"

func (q *QRegister) Type() object.ObjectType { return qRegisterObjType }
func (q *QRegister) Inspect() string {
	return fmt.Sprintf("quantum register: %d qubits (LOCAL SIMULATOR — not quantum hardware)", q.backend.NumQubits())
}

// ---------------------------------------------------------------------------
// localSimulator — the honest local state-vector implementation.
// ---------------------------------------------------------------------------

// gateOp records one applied gate for the ASCII circuit diagram.
type gateOp struct {
	name    string // "H", "X", "Y", "Z", "S", "T", "RX", "RY", "RZ", "CNOT", "CZ", "SWAP", "M"
	targets []int
	param   float64 // rotation angle (radians), for RX/RY/RZ
	hasParm bool
}

// localSimulator tracks 2^n complex amplitudes and applies real gate
// matrices to them. Ideal and noise-free: no decoherence, no gate errors,
// no shot noise beyond the classical randomness of measurement collapse.
type localSimulator struct {
	n     int
	state []complex128
	ops   []gateOp
}

// newLocalSimulator allocates an n-qubit register in |0...0>.
func newLocalSimulator(n int) (*localSimulator, error) {
	if n < 1 {
		return nil, fmt.Errorf("qalloc: need at least 1 qubit, got %d", n)
	}
	if n > maxSimQubits {
		return nil, fmt.Errorf("simulator limit: at most %d qubits (2^%d amplitudes = 256MB of state); %d qubits refused",
			maxSimQubits, maxSimQubits, n)
	}
	s := &localSimulator{n: n, state: make([]complex128, 1<<uint(n))}
	s.state[0] = 1 // |0...0>
	return s, nil
}

var _ QuantumBackend = (*localSimulator)(nil)

func (s *localSimulator) BackendName() string { return "local-simulator" }
func (s *localSimulator) NumQubits() int      { return s.n }

// simQubitRange validates a qubit index.
func (s *localSimulator) simQubitRange(op string, i int) error {
	if i < 0 || i >= s.n {
		return fmt.Errorf("%s: qubit index %d out of range (register has %d qubits, indices 0..%d)",
			op, i, s.n, s.n-1)
	}
	return nil
}

// applySingle applies a 2x2 unitary to qubit i. Little-endian: qubit i
// toggles with stride 2^i over the state vector.
func (s *localSimulator) applySingle(i int, m [2][2]complex128) {
	stride := 1 << uint(i)
	dim := len(s.state)
	for base := 0; base < dim; base += 2 * stride {
		for j := 0; j < stride; j++ {
			a0 := s.state[base+j]
			a1 := s.state[base+j+stride]
			s.state[base+j] = m[0][0]*a0 + m[0][1]*a1
			s.state[base+j+stride] = m[1][0]*a0 + m[1][1]*a1
		}
	}
}

// applyCNOT flips the target qubit's amplitude pairing wherever the
// control qubit is 1.
func (s *localSimulator) applyCNOT(ctrl, tgt int) {
	mask := 1 << uint(tgt)
	dim := len(s.state)
	for i := 0; i < dim; i++ {
		if i&(1<<uint(ctrl)) != 0 {
			j := i ^ mask
			if i < j {
				s.state[i], s.state[j] = s.state[j], s.state[i]
			}
		}
	}
}

// applyCZ applies a -1 phase wherever both control and target are 1.
func (s *localSimulator) applyCZ(a, b int) {
	dim := len(s.state)
	for i := 0; i < dim; i++ {
		if i&(1<<uint(a)) != 0 && i&(1<<uint(b)) != 0 {
			s.state[i] = -s.state[i]
		}
	}
}

// applySwap exchanges the amplitudes of the two qubits wherever they differ.
func (s *localSimulator) applySwap(a, b int) {
	mask := (1 << uint(a)) | (1 << uint(b))
	dim := len(s.state)
	for i := 0; i < dim; i++ {
		ba := (i >> uint(a)) & 1
		bb := (i >> uint(b)) & 1
		if ba != bb {
			j := i ^ mask
			if i < j {
				s.state[i], s.state[j] = s.state[j], s.state[i]
			}
		}
	}
}

// Single-qubit gate matrices.
var (
	gateH = [2][2]complex128{{1 / math.Sqrt2, 1 / math.Sqrt2}, {1 / math.Sqrt2, -1 / math.Sqrt2}}
	gateX = [2][2]complex128{{0, 1}, {1, 0}}
	gateY = [2][2]complex128{{0, -1i}, {1i, 0}}
	gateZ = [2][2]complex128{{1, 0}, {0, -1}}
	gateS = [2][2]complex128{{1, 0}, {0, 1i}}
	gateT = [2][2]complex128{{1, 0}, {0, cmplx.Exp(complex(0, math.Pi/4))}}
)

// rotationMatrix builds RX/RY/RZ from the angle in radians.
func rotationMatrix(axis string, theta float64) [2][2]complex128 {
	c := complex(math.Cos(theta/2), 0)
	s := complex(math.Sin(theta/2), 0)
	switch axis {
	case "RX":
		return [2][2]complex128{{c, -1i * s}, {-1i * s, c}}
	case "RY":
		return [2][2]complex128{{c, -s}, {s, c}}
	default: // RZ
		return [2][2]complex128{{cmplx.Exp(complex(0, -theta/2)), 0}, {0, cmplx.Exp(complex(0, theta/2))}}
	}
}

// ApplyGate implements QuantumBackend.
func (s *localSimulator) ApplyGate(name string, targets []int, params []float64) error {
	// Validate all target indices first — a gate never half-applies.
	for _, t := range targets {
		if err := s.simQubitRange("q_"+strings.ToLower(name), t); err != nil {
			return err
		}
	}
	switch name {
	case "H", "X", "Y", "Z", "S", "T":
		if len(targets) != 1 {
			return fmt.Errorf("q_%s: want 1 target qubit", strings.ToLower(name))
		}
		mats := map[string][2][2]complex128{"H": gateH, "X": gateX, "Y": gateY, "Z": gateZ, "S": gateS, "T": gateT}
		s.applySingle(targets[0], mats[name])
		s.ops = append(s.ops, gateOp{name: name, targets: []int{targets[0]}})
		return nil
	case "RX", "RY", "RZ":
		if len(targets) != 1 || len(params) != 1 {
			return fmt.Errorf("q_%s: want 1 target qubit and 1 angle (radians)", strings.ToLower(name))
		}
		s.applySingle(targets[0], rotationMatrix(name, params[0]))
		s.ops = append(s.ops, gateOp{name: name, targets: []int{targets[0]}, param: params[0], hasParm: true})
		return nil
	case "CNOT", "CZ":
		if len(targets) != 2 {
			return fmt.Errorf("q_%s: want control, target qubits", strings.ToLower(name))
		}
		if targets[0] == targets[1] {
			return fmt.Errorf("q_%s: control and target must be different qubits", strings.ToLower(name))
		}
		if name == "CNOT" {
			s.applyCNOT(targets[0], targets[1])
		} else {
			s.applyCZ(targets[0], targets[1])
		}
		s.ops = append(s.ops, gateOp{name: name, targets: []int{targets[0], targets[1]}})
		return nil
	case "SWAP":
		if len(targets) != 2 {
			return fmt.Errorf("q_swap: want 2 qubits")
		}
		if targets[0] == targets[1] {
			return fmt.Errorf("q_swap: the two qubits must differ")
		}
		s.applySwap(targets[0], targets[1])
		s.ops = append(s.ops, gateOp{name: name, targets: []int{targets[0], targets[1]}})
		return nil
	default:
		return fmt.Errorf("unknown gate %q (the local simulator knows H X Y Z S T RX RY RZ CNOT CZ SWAP)", name)
	}
}

// probOfOne returns the total probability that qubit i measures 1.
func (s *localSimulator) probOfOne(i int) float64 {
	p := 0.0
	for idx, amp := range s.state {
		if idx&(1<<uint(i)) != 0 {
			p += real(amp)*real(amp) + imag(amp)*imag(amp)
		}
	}
	return p
}

// cryptoUnit draws a uniform float64 in [0,1) from crypto/rand — honest
// classical randomness for measurement collapse, not a fixed seed.
func cryptoUnit() (float64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return float64(binary.BigEndian.Uint64(b[:])) / (1 << 64), nil
}

// Measure implements QuantumBackend: real probabilistic collapse.
func (s *localSimulator) Measure(qubit int) (int, error) {
	if err := s.simQubitRange("q_measure", qubit); err != nil {
		return 0, err
	}
	p1 := s.probOfOne(qubit)
	r, err := cryptoUnit()
	if err != nil {
		return 0, fmt.Errorf("q_measure: randomness source failed: %s", err.Error())
	}
	outcome := 0
	if r < p1 {
		outcome = 1
	}
	// Collapse: zero out the inconsistent branch, renormalize the other.
	norm := math.Sqrt(p1)
	if outcome == 0 {
		norm = math.Sqrt(1 - p1)
	}
	if norm == 0 {
		// Numerically impossible branch was (impossibly) chosen; this
		// cannot happen since r < p1 with p1 == 0 is false and r < 1
		// with p1 == 1 is true. Guard anyway — never silently wrong.
		return 0, fmt.Errorf("q_measure: internal error: zero-probability outcome selected")
	}
	for i, amp := range s.state {
		if (i>>uint(qubit))&1 == outcome {
			s.state[i] = amp / complex(norm, 0)
		} else {
			s.state[i] = 0
		}
	}
	s.ops = append(s.ops, gateOp{name: "M", targets: []int{qubit}})
	return outcome, nil
}

// MeasureAll implements QuantumBackend.
func (s *localSimulator) MeasureAll() ([]int, error) {
	bits := make([]int, s.n)
	for i := 0; i < s.n; i++ {
		b, err := s.Measure(i)
		if err != nil {
			return nil, err
		}
		bits[i] = b
	}
	return bits, nil
}

// Probabilities implements QuantumBackend.
func (s *localSimulator) Probabilities() []float64 {
	p := make([]float64, len(s.state))
	for i, amp := range s.state {
		p[i] = real(amp)*real(amp) + imag(amp)*imag(amp)
	}
	return p
}

// Amplitudes implements QuantumBackend (returns a copy).
func (s *localSimulator) Amplitudes() []complex128 {
	cp := make([]complex128, len(s.state))
	copy(cp, s.state)
	return cp
}

// Reset implements QuantumBackend.
func (s *localSimulator) Reset() {
	for i := range s.state {
		s.state[i] = 0
	}
	s.state[0] = 1
	s.ops = nil
}

// CircuitDiagram implements QuantumBackend: a readable ASCII diagram,
// one row per qubit, one column per recorded gate.
func (s *localSimulator) CircuitDiagram() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("quantum circuit — %d qubit(s) — LOCAL SIMULATOR (not quantum hardware)\n", s.n))
	if len(s.ops) == 0 {
		b.WriteString("(no gates applied yet — register is in |0...0>)\n")
		return b.String()
	}
	// Column width 4: "─H──", "─●──", "─⊕──", "─×──", "─M──", "─Rx─", "────".
	cell := func(sym string) string {
		// sym is 1-2 runes; pad with ─ to width 4.
		w := 0
		for range sym {
			w++
		}
		pad := 4 - w
		left := 1
		right := pad - left
		return strings.Repeat("─", left) + sym + strings.Repeat("─", right)
	}
	rows := make([]strings.Builder, s.n)
	for i := range rows {
		fmt.Fprintf(&rows[i], "q%d: ", i)
	}
	for _, op := range s.ops {
		for q := 0; q < s.n; q++ {
			sym := "──" // plain wire
			for ti, t := range op.targets {
				if t != q {
					continue
				}
				switch op.name {
				case "CNOT":
					if ti == 0 {
						sym = "●"
					} else {
						sym = "⊕"
					}
				case "CZ":
					sym = "◉"
				case "SWAP":
					sym = "×"
				case "M":
					sym = "M"
				default:
					sym = op.name
					if op.hasParm {
						// RX/RY/RZ show as Rx/Ry/Rz (angle lives in the op list below).
						sym = strings.ToLower(op.name[:1]) + op.name[1:]
					}
				}
			}
			rows[q].WriteString(cell(sym))
		}
	}
	for i := range rows {
		b.WriteString(rows[i].String() + "\n")
	}
	// Op legend with full detail (angles, control→target order).
	b.WriteString("ops: ")
	parts := make([]string, 0, len(s.ops))
	for _, op := range s.ops {
		var sb strings.Builder
		sb.WriteString(op.name)
		if op.hasParm {
			fmt.Fprintf(&sb, "(%g)", op.param)
		}
		sb.WriteString("(")
		sep := ","
		if op.name == "CNOT" || op.name == "CZ" {
			sep = "→"
		}
		for ti, t := range op.targets {
			if ti > 0 {
				sb.WriteString(sep)
			}
			fmt.Fprintf(&sb, "q%d", t)
		}
		sb.WriteString(")")
		parts = append(parts, sb.String())
	}
	b.WriteString(strings.Join(parts, " ") + "\n")
	return b.String()
}

// ---------------------------------------------------------------------------
// Builtin plumbing.
// ---------------------------------------------------------------------------

// wave8Reg extracts the *QRegister from the first argument.
func wave8Reg(obj object.Object, fname string) (*QRegister, object.Object) {
	q, ok := obj.(*QRegister)
	if !ok {
		return nil, newError("%s: first argument must be a quantum register from qalloc(), got %s", fname, obj.Type())
	}
	return q, nil
}

// wave8Qubit validates a qubit index argument.
func wave8Qubit(q *QRegister, obj object.Object, fname string) (int, object.Object) {
	i, ok := obj.(*object.Integer)
	if !ok {
		return 0, newError("%s: qubit index must be an integer, got %s", fname, obj.Type())
	}
	if err := q.backend.(*localSimulator).simQubitRange(fname, int(i.Value)); err != nil {
		return 0, newError("%s", err.Error())
	}
	return int(i.Value), nil
}

// wave8Theta validates a rotation angle (int or float, radians).
func wave8Theta(obj object.Object, fname string) (float64, object.Object) {
	switch t := obj.(type) {
	case *object.Integer:
		return float64(t.Value), nil
	case *object.Float:
		return t.Value, nil
	default:
		return 0, newError("%s: angle must be a number (radians), got %s", fname, obj.Type())
	}
}

// wave8SingleGate builds a builtin for a one-qubit gate.
func wave8SingleGate(fname, gate string) *object.Builtin {
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("%s: want quantum register, qubit index", fname)
			}
			q, errObj := wave8Reg(args[0], fname)
			if errObj != nil {
				return errObj
			}
			i, errObj := wave8Qubit(q, args[1], fname)
			if errObj != nil {
				return errObj
			}
			if err := q.backend.ApplyGate(gate, []int{i}, nil); err != nil {
				return newError("%s", err.Error())
			}
			return q
		},
	}
}

// wave8TwoQubitGate builds a builtin for a two-qubit gate.
func wave8TwoQubitGate(fname, gate string) *object.Builtin {
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("%s: want quantum register, qubit, qubit", fname)
			}
			q, errObj := wave8Reg(args[0], fname)
			if errObj != nil {
				return errObj
			}
			a, errObj := wave8Qubit(q, args[1], fname)
			if errObj != nil {
				return errObj
			}
			b, errObj := wave8Qubit(q, args[2], fname)
			if errObj != nil {
				return errObj
			}
			if err := q.backend.ApplyGate(gate, []int{a, b}, nil); err != nil {
				return newError("%s", err.Error())
			}
			return q
		},
	}
}

// wave8RotationGate builds a builtin for q_rx/q_ry/q_rz.
func wave8RotationGate(fname, gate string) *object.Builtin {
	return &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 3 {
				return newError("%s: want quantum register, qubit index, angle in radians", fname)
			}
			q, errObj := wave8Reg(args[0], fname)
			if errObj != nil {
				return errObj
			}
			i, errObj := wave8Qubit(q, args[1], fname)
			if errObj != nil {
				return errObj
			}
			theta, errObj := wave8Theta(args[2], fname)
			if errObj != nil {
				return errObj
			}
			if err := q.backend.ApplyGate(gate, []int{i}, []float64{theta}); err != nil {
				return newError("%s", err.Error())
			}
			return q
		},
	}
}

// registerWave8Builtins adds the wave-8 quantum builtins. Called once from
// initBuiltins() (same pattern as registerWave7Builtins).
func registerWave8Builtins() {
	// ---- allocation ----
	builtins["qalloc"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("qalloc: want number of qubits")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return newError("qalloc: number of qubits must be an integer, got %s", args[0].Type())
			}
			sim, err := newLocalSimulator(int(n.Value))
			if err != nil {
				return newError("%s", err.Error())
			}
			return &QRegister{backend: sim}
		},
	}

	// ---- single-qubit gates (return the register, for chaining) ----
	builtins["q_h"] = wave8SingleGate("q_h", "H")
	builtins["q_x"] = wave8SingleGate("q_x", "X")
	builtins["q_y"] = wave8SingleGate("q_y", "Y")
	builtins["q_z"] = wave8SingleGate("q_z", "Z")
	builtins["q_s"] = wave8SingleGate("q_s", "S")
	builtins["q_t"] = wave8SingleGate("q_t", "T")

	// ---- rotations (angle in radians) ----
	builtins["q_rx"] = wave8RotationGate("q_rx", "RX")
	builtins["q_ry"] = wave8RotationGate("q_ry", "RY")
	builtins["q_rz"] = wave8RotationGate("q_rz", "RZ")

	// ---- two-qubit gates ----
	builtins["q_cnot"] = wave8TwoQubitGate("q_cnot", "CNOT")
	builtins["q_cz"] = wave8TwoQubitGate("q_cz", "CZ")
	builtins["q_swap"] = wave8TwoQubitGate("q_swap", "SWAP")

	// ---- measurement: real probabilistic collapse via crypto/rand ----
	builtins["q_measure"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("q_measure: want quantum register, qubit index")
			}
			q, errObj := wave8Reg(args[0], "q_measure")
			if errObj != nil {
				return errObj
			}
			i, errObj := wave8Qubit(q, args[1], "q_measure")
			if errObj != nil {
				return errObj
			}
			b, err := q.backend.Measure(i)
			if err != nil {
				return newError("%s", err.Error())
			}
			return &object.Integer{Value: int64(b)}
		},
	}
	builtins["q_measure_all"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_measure_all: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_measure_all")
			if errObj != nil {
				return errObj
			}
			bits, err := q.backend.MeasureAll()
			if err != nil {
				return newError("%s", err.Error())
			}
			// Array of bits in qubit order: element 0 is qubit 0.
			els := make([]object.Object, len(bits))
			for i, b := range bits {
				els[i] = &object.Integer{Value: int64(b)}
			}
			return &object.Array{Elements: els}
		},
	}

	// ---- inspection (deterministic — the test-friendly surface) ----
	builtins["q_probs"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_probs: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_probs")
			if errObj != nil {
				return errObj
			}
			probs := q.backend.Probabilities()
			els := make([]object.Object, len(probs))
			for i, p := range probs {
				els[i] = &object.Float{Value: p}
			}
			return &object.Array{Elements: els}
		},
	}
	builtins["q_state"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_state: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_state")
			if errObj != nil {
				return errObj
			}
			// NvS has no complex type: each amplitude is a [re, im] pair.
			amps := q.backend.Amplitudes()
			els := make([]object.Object, len(amps))
			for i, a := range amps {
				els[i] = &object.Array{Elements: []object.Object{
					&object.Float{Value: real(a)},
					&object.Float{Value: imag(a)},
				}}
			}
			return &object.Array{Elements: els}
		},
	}
	builtins["q_nqubits"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_nqubits: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_nqubits")
			if errObj != nil {
				return errObj
			}
			return &object.Integer{Value: int64(q.backend.NumQubits())}
		},
	}

	// ---- circuit display + reset ----
	builtins["q_circuit"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_circuit: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_circuit")
			if errObj != nil {
				return errObj
			}
			return &object.String{Value: q.backend.CircuitDiagram()}
		},
	}
	builtins["q_reset"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_reset: want quantum register")
			}
			q, errObj := wave8Reg(args[0], "q_reset")
			if errObj != nil {
				return errObj
			}
			q.backend.Reset()
			return q
		},
	}

	// ---- backend registry: honest about what this build can reach ----
	builtins["q_backend"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_backend: want backend name (e.g. \"local\")")
			}
			name, ok := args[0].(*object.String)
			if !ok {
				return newError("q_backend: backend name must be a string, got %s", args[0].Type())
			}
			switch strings.ToLower(strings.TrimSpace(name.Value)) {
			case "local":
				return &object.String{Value: "local-simulator"}
			default:
				// Never fake a hardware connection: an unconnected
				// backend is a loud error, not a silent simulator.
				return newError("hardware backend %q is not connected in this build — local simulator only (a future Azure Quantum backend would implement the QuantumBackend interface)", name.Value)
			}
		},
	}
}
