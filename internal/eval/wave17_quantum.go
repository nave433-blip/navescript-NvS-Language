// Wave 17f: deeper quantum — Toffoli, controlled phase, seeded shots,
// and a chainable circuit-builder object.
//
// SIMULATED QUANTUM COMPUTING DISCLAIMER: every builtin here drives the
// LOCAL classical state-vector simulator from wave 8. Nothing touches
// quantum hardware; gates are matrix algebra over complex128 amplitudes
// and "measurement" is sampling from a classical probability
// distribution. Ideal and noise-free: no decoherence, no gate errors.
package eval

import (
	"fmt"
	"math"
	"math/cmplx"
	"math/rand"
	"strings"
	"time"

	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/quantum"
)

// wave17MeasRand is the seedable classical RNG behind wave-17 measurement
// helpers. q_seed() reseeds it for reproducible demos; q_shots() and the
// circuit builder's measure()/measure_all() draw from it. It is NOT safe
// for concurrent use from multiple tasks.
var wave17MeasRand = rand.New(rand.NewSource(time.Now().UnixNano()))

// wave17Sim extracts the *localSimulator backend from a QRegister for
// wave-17 builtins that manipulate state bits directly.
func wave17Sim(q *QRegister, fname string) (*localSimulator, object.Object) {
	sim, ok := q.backend.(*localSimulator)
	if !ok {
		return nil, newError("%s: requires the local state-vector simulator backend, got %s (SIMULATED quantum computing — classical, not hardware)", fname, q.backend.BackendName())
	}
	return sim, nil
}

// wave17Qubit validates a qubit index argument against the simulator.
func wave17Qubit(sim *localSimulator, obj object.Object, fname string) (int, object.Object) {
	i, ok := obj.(*object.Integer)
	if !ok {
		return 0, newError("%s: qubit index must be an integer, got %s", fname, obj.Type())
	}
	if err := sim.simQubitRange(fname, int(i.Value)); err != nil {
		return 0, newError("%s", err.Error())
	}
	return int(i.Value), nil
}

// wave17Distinct validates that the given qubit indices are all different.
func wave17Distinct(fname string, qs ...int) object.Object {
	for i := 0; i < len(qs); i++ {
		for j := i + 1; j < len(qs); j++ {
			if qs[i] == qs[j] {
				return newError("%s: qubit indices must be distinct, got %d twice", fname, qs[i])
			}
		}
	}
	return nil
}

// wave17ApplyToffoli flips target t wherever both controls c1 and c2 are 1.
// Little-endian state layout: qubit i toggles with stride 2^i.
func wave17ApplyToffoli(sim *localSimulator, c1, c2, t int) {
	tmask := 1 << uint(t)
	cmask := (1 << uint(c1)) | (1 << uint(c2))
	for i := 0; i < len(sim.state); i++ {
		if i&cmask == cmask {
			j := i ^ tmask
			if i < j {
				sim.state[i], sim.state[j] = sim.state[j], sim.state[i]
			}
		}
	}
	sim.ops = append(sim.ops, gateOp{name: "TOFFOLI", targets: []int{c1, c2, t}})
}

// wave17ApplyCPhase multiplies amplitudes by e^{i*phi} wherever both
// control c and target t are 1.
func wave17ApplyCPhase(sim *localSimulator, c, t int, phi float64) {
	factor := cmplx.Exp(complex(0, phi))
	for i, amp := range sim.state {
		if i&(1<<uint(c)) != 0 && i&(1<<uint(t)) != 0 {
			sim.state[i] = amp * factor
		}
	}
	sim.ops = append(sim.ops, gateOp{name: "CPHASE", targets: []int{c, t}, param: phi, hasParm: true})
}

// wave17SeededCollapse measures one qubit using rng instead of the
// crypto/rand source that q_measure uses, so q_seed() makes demos
// reproducible. Still a LOCAL CLASSICAL SIMULATION of collapse.
func wave17SeededCollapse(sim *localSimulator, qubit int, rng *rand.Rand) int {
	p1 := sim.probOfOne(qubit)
	outcome := 0
	if rng.Float64() < p1 {
		outcome = 1
	}
	norm := math.Sqrt(p1)
	if outcome == 0 {
		norm = math.Sqrt(1 - p1)
	}
	if norm > 0 {
		for i, amp := range sim.state {
			if (i>>uint(qubit))&1 == outcome {
				sim.state[i] = amp / complex(norm, 0)
			} else {
				sim.state[i] = 0
			}
		}
	}
	sim.ops = append(sim.ops, gateOp{name: "M", targets: []int{qubit}})
	return outcome
}

// wave17Sample draws one outcome index from the full-state distribution
// WITHOUT collapsing the state.
func wave17Sample(sim *localSimulator, rng *rand.Rand) int {
	probs := sim.Probabilities()
	r := rng.Float64()
	var acc float64
	out := len(probs) - 1
	for i, p := range probs {
		acc += p
		if r <= acc {
			out = i
			break
		}
	}
	return out
}

// wave17Hist builds an NvS hash of outcome(int) -> count(int).
func wave17Hist(hist map[int]int) *object.Hash {
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	for outcome, count := range hist {
		k := &object.Integer{Value: int64(outcome)}
		h.Pairs[k.HashKey()] = object.HashPair{Key: k, Value: &object.Integer{Value: int64(count)}}
	}
	return h
}

// ---------------------------------------------------------------------------
// wave17Circuit — chainable circuit-builder object.
// ---------------------------------------------------------------------------

// wave17Circuit wraps a local simulator and a quantum.Op log for the
// ASCII diagram. Every mutating method returns the builder hash itself so
// calls chain: circuit(2).h(0).cnot(0,1).draw().
//
// SIMULATED QUANTUM COMPUTING: this builds circuits for the LOCAL
// classical state-vector simulator, not for quantum hardware.
type wave17Circuit struct {
	n   int
	sim *localSimulator
	ops []quantum.Op
	h   *object.Hash
}

func wave17CircuitQubit(c *wave17Circuit, fname string, obj object.Object) (int, object.Object) {
	return wave17Qubit(c.sim, obj, fname)
}

func wave17NewCircuit(n int64) (object.Object, object.Object) {
	sim, err := newLocalSimulator(int(n))
	if err != nil {
		return nil, newError("%s", err.Error())
	}
	c := &wave17Circuit{n: int(n), sim: sim}
	h := &object.Hash{Pairs: map[object.HashKey]object.HashPair{}}
	c.h = h
	add := func(name string, b *object.Builtin) {
		key := &object.String{Value: name}
		h.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: b}
	}
	ret := func() object.Object { return c.h }

	single := func(mname, gate string) {
		add(mname, &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("circuit.%s: want qubit index", mname)
			}
			q, errObj := wave17CircuitQubit(c, "circuit."+mname, args[0])
			if errObj != nil {
				return errObj
			}
			if err := c.sim.ApplyGate(gate, []int{q}, nil); err != nil {
				return newError("%s", err.Error())
			}
			c.ops = append(c.ops, quantum.Op{Name: gate, Qubits: []int{q}})
			return ret()
		}})
	}
	for _, g := range [][2]string{{"h", "H"}, {"x", "X"}, {"y", "Y"}, {"z", "Z"}, {"s", "S"}, {"t", "T"}} {
		single(g[0], g[1])
	}

	rot := func(mname, gate string) {
		add(mname, &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("circuit.%s: want qubit index, angle in radians", mname)
			}
			q, errObj := wave17CircuitQubit(c, "circuit."+mname, args[0])
			if errObj != nil {
				return errObj
			}
			theta, errObj := wave8Theta(args[1], "circuit."+mname)
			if errObj != nil {
				return errObj
			}
			if err := c.sim.ApplyGate(gate, []int{q}, []float64{theta}); err != nil {
				return newError("%s", err.Error())
			}
			c.ops = append(c.ops, quantum.Op{Name: gate, Qubits: []int{q}, Param: theta, HasParam: true})
			return ret()
		}})
	}
	rot("rx", "RX")
	rot("ry", "RY")
	rot("rz", "RZ")

	pair := func(mname, gate string) {
		add(mname, &object.Builtin{Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("circuit.%s: want control, target qubit indices", mname)
			}
			a, errObj := wave17CircuitQubit(c, "circuit."+mname, args[0])
			if errObj != nil {
				return errObj
			}
			b, errObj := wave17CircuitQubit(c, "circuit."+mname, args[1])
			if errObj != nil {
				return errObj
			}
			if errObj := wave17Distinct("circuit."+mname, a, b); errObj != nil {
				return errObj
			}
			if err := c.sim.ApplyGate(gate, []int{a, b}, nil); err != nil {
				return newError("%s", err.Error())
			}
			c.ops = append(c.ops, quantum.Op{Name: gate, Qubits: []int{a, b}})
			return ret()
		}})
	}
	pair("cnot", "CNOT")
	pair("cz", "CZ")
	pair("swap", "SWAP")

	add("toffoli", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 3 {
			return newError("circuit.toffoli: want control1, control2, target qubit indices")
		}
		qs := make([]int, 3)
		for i, a := range args {
			q, errObj := wave17CircuitQubit(c, "circuit.toffoli", a)
			if errObj != nil {
				return errObj
			}
			qs[i] = q
		}
		if errObj := wave17Distinct("circuit.toffoli", qs...); errObj != nil {
			return errObj
		}
		wave17ApplyToffoli(c.sim, qs[0], qs[1], qs[2])
		c.ops = append(c.ops, quantum.Op{Name: "TOFFOLI", Qubits: qs})
		return ret()
	}})

	add("cphase", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 3 {
			return newError("circuit.cphase: want control, target qubit indices, phase angle in radians")
		}
		a, errObj := wave17CircuitQubit(c, "circuit.cphase", args[0])
		if errObj != nil {
			return errObj
		}
		b, errObj := wave17CircuitQubit(c, "circuit.cphase", args[1])
		if errObj != nil {
			return errObj
		}
		if errObj := wave17Distinct("circuit.cphase", a, b); errObj != nil {
			return errObj
		}
		phi, errObj := wave8Theta(args[2], "circuit.cphase")
		if errObj != nil {
			return errObj
		}
		wave17ApplyCPhase(c.sim, a, b, phi)
		c.ops = append(c.ops, quantum.Op{Name: "CPHASE", Qubits: []int{a, b}, Param: phi, HasParam: true})
		return ret()
	}})

	// ---- seeded measurement (reproducible after q_seed) ----
	add("measure", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return newError("circuit.measure: want qubit index")
		}
		q, errObj := wave17CircuitQubit(c, "circuit.measure", args[0])
		if errObj != nil {
			return errObj
		}
		out := wave17SeededCollapse(c.sim, q, wave17MeasRand)
		c.ops = append(c.ops, quantum.Op{Name: "MEASURE", Qubits: []int{q}})
		return &object.Integer{Value: int64(out)}
	}})

	add("measure_all", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return newError("circuit.measure_all: want no arguments")
		}
		els := make([]object.Object, c.n)
		for i := 0; i < c.n; i++ {
			els[i] = &object.Integer{Value: int64(wave17SeededCollapse(c.sim, i, wave17MeasRand))}
		}
		c.ops = append(c.ops, quantum.Op{Name: "MEASURE", Qubits: []int{0}})
		return &object.Array{Elements: els}
	}})

	// ---- inspection ----
	add("probs", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return newError("circuit.probs: want no arguments")
		}
		probs := c.sim.Probabilities()
		els := make([]object.Object, len(probs))
		for i, p := range probs {
			els[i] = &object.Float{Value: p}
		}
		return &object.Array{Elements: els}
	}})

	add("state", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return newError("circuit.state: want no arguments")
		}
		amps := c.sim.Amplitudes()
		els := make([]object.Object, len(amps))
		for i, a := range amps {
			els[i] = &object.Array{Elements: []object.Object{
				&object.Float{Value: real(a)},
				&object.Float{Value: imag(a)},
			}}
		}
		return &object.Array{Elements: els}
	}})

	add("draw", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return newError("circuit.draw: want no arguments")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("quantum circuit — %d qubit(s) — LOCAL SIMULATOR (not quantum hardware)\n", c.n))
		sb.WriteString(quantum.DrawCircuit(c.n, c.ops))
		return &object.String{Value: sb.String()}
	}})

	// run(shots, seed): sample shots outcomes WITHOUT collapsing the
	// builder state, using an explicit seed for reproducibility.
	add("run", &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 2 {
			return newError("circuit.run: want shot count, seed")
		}
		nshots, ok := args[0].(*object.Integer)
		if !ok || nshots.Value < 1 {
			return newError("circuit.run: shot count must be a positive integer, got %s", args[0].Inspect())
		}
		seed, ok := args[1].(*object.Integer)
		if !ok {
			return newError("circuit.run: seed must be an integer, got %s", args[1].Type())
		}
		rng := rand.New(rand.NewSource(seed.Value))
		hist := make(map[int]int)
		for k := int64(0); k < nshots.Value; k++ {
			hist[wave17Sample(c.sim, rng)]++
		}
		return wave17Hist(hist)
	}})

	return h, nil
}

// ---------------------------------------------------------------------------
// Builtin registration.
// ---------------------------------------------------------------------------

// registerWave17QuantumBuiltins adds the wave-17 deeper-quantum builtins:
// q_toffoli, q_cphase, q_seed, q_shots, and the circuit() builder.
//
// SIMULATED QUANTUM COMPUTING DISCLAIMER: all of these drive the LOCAL
// classical state-vector simulator — matrix algebra over complex128
// amplitudes, never quantum hardware.
func registerWave17QuantumBuiltins() {
	builtins["q_toffoli"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 4 {
				return newError("q_toffoli: want quantum register, control1, control2, target qubit indices")
			}
			q, errObj := wave8Reg(args[0], "q_toffoli")
			if errObj != nil {
				return errObj
			}
			sim, errObj := wave17Sim(q, "q_toffoli")
			if errObj != nil {
				return errObj
			}
			qs := make([]int, 3)
			for i, a := range args[1:] {
				idx, errObj := wave17Qubit(sim, a, "q_toffoli")
				if errObj != nil {
					return errObj
				}
				qs[i] = idx
			}
			if errObj := wave17Distinct("q_toffoli", qs...); errObj != nil {
				return errObj
			}
			wave17ApplyToffoli(sim, qs[0], qs[1], qs[2])
			return q
		},
	}

	builtins["q_cphase"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 4 {
				return newError("q_cphase: want quantum register, control, target qubit indices, phase angle in radians")
			}
			q, errObj := wave8Reg(args[0], "q_cphase")
			if errObj != nil {
				return errObj
			}
			sim, errObj := wave17Sim(q, "q_cphase")
			if errObj != nil {
				return errObj
			}
			c, errObj := wave17Qubit(sim, args[1], "q_cphase")
			if errObj != nil {
				return errObj
			}
			t, errObj := wave17Qubit(sim, args[2], "q_cphase")
			if errObj != nil {
				return errObj
			}
			if errObj := wave17Distinct("q_cphase", c, t); errObj != nil {
				return errObj
			}
			phi, errObj := wave8Theta(args[3], "q_cphase")
			if errObj != nil {
				return errObj
			}
			wave17ApplyCPhase(sim, c, t, phi)
			return q
		},
	}

	builtins["q_seed"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("q_seed: want an integer seed")
			}
			seed, ok := args[0].(*object.Integer)
			if !ok {
				return newError("q_seed: seed must be an integer, got %s", args[0].Type())
			}
			wave17MeasRand = rand.New(rand.NewSource(seed.Value))
			quantum.Seeded(seed.Value)
			return &object.Integer{Value: seed.Value}
		},
	}

	// q_shots(reg, n): sample n measurement outcomes WITHOUT collapsing
	// the register — the state is left untouched, so q_shots is safe to
	// call repeatedly (e.g. after q_seed for identical histograms).
	builtins["q_shots"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("q_shots: want quantum register, shot count")
			}
			q, errObj := wave8Reg(args[0], "q_shots")
			if errObj != nil {
				return errObj
			}
			sim, errObj := wave17Sim(q, "q_shots")
			if errObj != nil {
				return errObj
			}
			n, ok := args[1].(*object.Integer)
			if !ok || n.Value < 1 {
				return newError("q_shots: shot count must be a positive integer, got %s", args[1].Inspect())
			}
			hist := make(map[int]int)
			for k := int64(0); k < n.Value; k++ {
				hist[wave17Sample(sim, wave17MeasRand)]++
			}
			return wave17Hist(hist)
		},
	}

	builtins["circuit"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("circuit: want number of qubits")
			}
			n, ok := args[0].(*object.Integer)
			if !ok {
				return newError("circuit: number of qubits must be an integer, got %s", args[0].Type())
			}
			c, errObj := wave17NewCircuit(n.Value)
			if errObj != nil {
				return errObj
			}
			return c
		},
	}
}
