// Package quantum provides classical simulation of small quantum circuits
// (state vectors over C^2^n) for NvS educational / research scripting.
package quantum

import (
	"fmt"
	"math"
	"math/cmplx"
	"math/rand"
	"strings"
)

// Complex pair helpers as [re, im] float64 pairs for interop with NvS arrays.

// State is a normalized complex amplitude vector (length power of 2).
type State []complex128

// NewQubit returns |0> or |1>.
func NewQubit(bit int) State {
	if bit != 0 {
		return State{0, 1}
	}
	return State{1, 0}
}

// NewZero returns |0...0> for n qubits.
func NewZero(n int) State {
	dim := 1 << n
	s := make(State, dim)
	s[0] = 1
	return s
}

// Normalize scales state to unit norm.
func Normalize(s State) State {
	var norm float64
	for _, a := range s {
		norm += real(a * cmplx.Conj(a))
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return s
	}
	out := make(State, len(s))
	for i, a := range s {
		out[i] = a / complex(norm, 0)
	}
	return out
}

// Probabilities returns measurement probabilities |amp|^2.
func Probabilities(s State) []float64 {
	p := make([]float64, len(s))
	for i, a := range s {
		p[i] = real(a * cmplx.Conj(a))
	}
	return p
}

// Measure collapses state; returns outcome index and new state.
func Measure(s State) (int, State) {
	p := Probabilities(s)
	r := rand.Float64()
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

// ApplyGate applies a full dim x dim unitary (row-major) to state.
func ApplyGate(s State, u [][]complex128) (State, error) {
	n := len(s)
	if len(u) != n {
		return nil, fmt.Errorf("gate size %d != state %d", len(u), n)
	}
	out := make(State, n)
	for i := 0; i < n; i++ {
		var sum complex128
		for j := 0; j < n; j++ {
			sum += u[i][j] * s[j]
		}
		out[i] = sum
	}
	return Normalize(out), nil
}

// Tensor product of two states.
func Tensor(a, b State) State {
	out := make(State, len(a)*len(b))
	for i, ai := range a {
		for j, bj := range b {
			out[i*len(b)+j] = ai * bj
		}
	}
	return out
}

// Inner product <a|b>
func Inner(a, b State) complex128 {
	if len(a) != len(b) {
		return 0
	}
	var s complex128
	for i := range a {
		s += cmplx.Conj(a[i]) * b[i]
	}
	return s
}

// --- Standard single-qubit gates ---

func matrix2(a, b, c, d complex128) [][]complex128 {
	return [][]complex128{{a, b}, {c, d}}
}

func Hadamard() [][]complex128 {
	s := complex(1/math.Sqrt(2), 0)
	return matrix2(s, s, s, -s)
}

func PauliX() [][]complex128 {
	return matrix2(0, 1, 1, 0)
}

func PauliY() [][]complex128 {
	return matrix2(0, -1i, 1i, 0)
}

func PauliZ() [][]complex128 {
	return matrix2(1, 0, 0, -1)
}

func Phase(phi float64) [][]complex128 {
	return matrix2(1, 0, 0, cmplx.Exp(complex(0, phi)))
}

func RX(theta float64) [][]complex128 {
	c := complex(math.Cos(theta/2), 0)
	s := complex(0, -math.Sin(theta/2))
	return matrix2(c, s, s, c)
}

func RY(theta float64) [][]complex128 {
	c := complex(math.Cos(theta/2), 0)
	s := complex(math.Sin(theta/2), 0)
	return matrix2(c, -s, s, c)
}

func RZ(theta float64) [][]complex128 {
	return matrix2(cmplx.Exp(complex(0, -theta/2)), 0, 0, cmplx.Exp(complex(0, theta/2)))
}

// CNOT on 2-qubit space (control q0, target q1) — basis |00>,|01>,|10>,|11>
func CNOT() [][]complex128 {
	u := make([][]complex128, 4)
	for i := range u {
		u[i] = make([]complex128, 4)
	}
	// |00> -> |00>, |01> -> |01>, |10> -> |11>, |11> -> |10>
	u[0][0] = 1
	u[1][1] = 1
	u[2][3] = 1
	u[3][2] = 1
	return u
}

// ApplyNamedGate applies a named 1-qubit gate to a 1-qubit state.
func ApplyNamedGate(s State, name string, angle float64) (State, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	var u [][]complex128
	switch name {
	case "h", "hadamard":
		u = Hadamard()
	case "x", "pauli_x", "not":
		u = PauliX()
	case "y", "pauli_y":
		u = PauliY()
	case "z", "pauli_z":
		u = PauliZ()
	case "s", "phase":
		u = Phase(math.Pi / 2)
	case "t":
		u = Phase(math.Pi / 4)
	case "rx":
		u = RX(angle)
	case "ry":
		u = RY(angle)
	case "rz":
		u = RZ(angle)
	case "cnot":
		u = CNOT()
	default:
		return nil, fmt.Errorf("unknown gate %q", name)
	}
	return ApplyGate(s, u)
}

// ToPairs converts state to [][2]float64 for NvS arrays.
func ToPairs(s State) [][2]float64 {
	out := make([][2]float64, len(s))
	for i, a := range s {
		out[i] = [2]float64{real(a), imag(a)}
	}
	return out
}

// FromPairs builds state from [][2]float64.
func FromPairs(pairs [][2]float64) State {
	s := make(State, len(pairs))
	for i, p := range pairs {
		s[i] = complex(p[0], p[1])
	}
	return Normalize(s)
}

// Physics constants (SI where applicable)
const (
	Pi       = math.Pi
	E        = math.E
	Hbar     = 1.054571817e-34 // J·s
	H        = 6.62607015e-34  // Planck
	C        = 299792458.0     // m/s
	G        = 6.67430e-11
	K_B      = 1.380649e-23
	E_CHARGE = 1.602176634e-19
	M_E      = 9.1093837015e-31
	M_P      = 1.67262192369e-27
	NA       = 6.02214076e23
	ALPHA    = 7.2973525693e-3 // fine structure
)

// GreekLetters maps common Greek letter names / glyphs to float values (math constants where meaningful).
var GreekLetters = map[string]float64{
	"α": ALPHA, "alpha": ALPHA,
	"β": 0, "beta": 0, // placeholder; user-assignable via env
	"γ": 0, "gamma": 0,
	"δ": 0, "delta": 0,
	"ε": 0, "epsilon": 0,
	"ζ": 0, "zeta": 0,
	"η": 0, "eta": 0,
	"θ": 0, "theta": 0,
	"ι": 0, "iota": 0,
	"κ": 0, "kappa": 0,
	"λ": 0, "lambda": 0,
	"μ": 0, "mu": 0,
	"ν": 0, "nu": 0,
	"ξ": 0, "xi": 0,
	"ο": 0, "omicron": 0,
	"π": Pi, "pi": Pi,
	"ρ": 0, "rho": 0,
	"σ": 0, "sigma": 0,
	"τ": math.Pi * 2, "tau": math.Pi * 2,
	"υ": 0, "upsilon": 0,
	"φ": (1 + math.Sqrt(5)) / 2, "phi": (1 + math.Sqrt(5)) / 2, // golden ratio
	"χ": 0, "chi": 0,
	"ψ": 0, "psi": 0,
	"ω": 0, "omega": 0,
	"Γ": 0, "Δ": 0, "Θ": 0, "Λ": 0, "Ξ": 0, "Π": Pi, "Σ": 0, "Φ": 0, "Ψ": 0, "Ω": 0,
	"ℏ": Hbar, "hbar": Hbar,
	"∞": math.Inf(1),
}
