package eval

// Ported from the NvS 2.2.0–2.9.0 track (remote master): the low-level
// quantum state-vector API and the physics-constants lexicon.
//
// Two quantum APIs now coexist, honestly documented:
//   - wave8.go: circuit-level API (qalloc, q_h, q_cnot, q_probs, ...)
//     with crypto/rand measurement and a pluggable backend interface.
//   - THIS file: state-vector primitives (qubit, qzero, qgate, qtensor,
//     qmeasure, qprob, qnormalize, qinner) that pass raw [re,im] pair
//     arrays around, plus physics_const() and π/ℏ/c/... globals.
//     Measurement here uses math/rand (Go auto-seeds it since 1.20).
//
// !!! LOCAL SIMULATOR, NOT QUANTUM HARDWARE — same disclaimer as wave8.

import (
	"fmt"
	"math"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
	"github.com/navescript/nvs/internal/quantum"
)

// registerPort29Builtins wires the 2.9-track builtins into the registry.
func registerPort29Builtins() {
	builtins["qubit"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			bit := 0
			if len(args) >= 1 {
				if i, ok := args[0].(*object.Integer); ok {
					bit = int(i.Value)
				}
			}
			return port29StateToArray(quantum.NewQubit(bit))
		},
	}
	builtins["qzero"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			n := 1
			if len(args) >= 1 {
				if i, ok := args[0].(*object.Integer); ok {
					n = int(i.Value)
				}
			}
			if n < 1 {
				n = 1
			}
			if n > 8 {
				return newError("qzero: max 8 qubits in this build")
			}
			return port29StateToArray(quantum.NewZero(n))
		},
	}
	builtins["qgate"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// qgate(state, "H"|"X"|"Y"|"Z"|"S"|"T"|"RX"|"RY"|"RZ"|"CNOT", [angle])
			if len(args) < 2 {
				return newError("qgate: want state, name, [angle]")
			}
			s, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			name, ok := args[1].(*object.String)
			if !ok {
				return newError("qgate: name must be string")
			}
			angle := 0.0
			if len(args) >= 3 {
				angle, _ = toFloat(args[2])
			}
			out, err := quantum.ApplyNamedGate(s, name.Value, angle)
			if err != nil {
				return newError("%s", err.Error())
			}
			return port29StateToArray(out)
		},
	}
	builtins["qmeasure"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("qmeasure: want state")
			}
			s, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			out, ns := quantum.Measure(s)
			// return {outcome: int, state: [[re,im], ...]}
			k1 := &object.String{Value: "outcome"}
			k2 := &object.String{Value: "state"}
			return &object.Hash{Pairs: map[object.HashKey]object.HashPair{
				k1.HashKey(): {Key: k1, Value: &object.Integer{Value: int64(out)}},
				k2.HashKey(): {Key: k2, Value: port29StateToArray(ns)},
			}}
		},
	}
	builtins["qprob"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) < 1 {
				return newError("qprob: want state")
			}
			s, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			p := quantum.Probabilities(s)
			els := make([]object.Object, len(p))
			for i, v := range p {
				els[i] = &object.Float{Value: v}
			}
			return &object.Array{Elements: els}
		},
	}
	builtins["qtensor"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("qtensor: want state_a, state_b")
			}
			a, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			b, err := port29ArrayToState(args[1])
			if err != nil {
				return newError("%s", err.Error())
			}
			return port29StateToArray(quantum.Tensor(a, b))
		},
	}
	builtins["qnormalize"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return newError("qnormalize: want state")
			}
			s, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			return port29StateToArray(quantum.Normalize(s))
		},
	}
	builtins["qinner"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			if len(args) != 2 {
				return newError("qinner: want state_a, state_b")
			}
			a, err := port29ArrayToState(args[0])
			if err != nil {
				return newError("%s", err.Error())
			}
			b, err := port29ArrayToState(args[1])
			if err != nil {
				return newError("%s", err.Error())
			}
			c := quantum.Inner(a, b)
			// return [re, im]
			return &object.Array{Elements: []object.Object{
				&object.Float{Value: real(c)},
				&object.Float{Value: imag(c)},
			}}
		},
	}
	builtins["physics_const"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// physics_const("hbar"|"pi"|"c"|...) or physics_const() to list all
			if len(args) == 0 {
				pairs := map[object.HashKey]object.HashPair{}
				for k, v := range quantum.GreekLetters {
					ks := &object.String{Value: k}
					vs := &object.Float{Value: v}
					pairs[ks.HashKey()] = object.HashPair{Key: ks, Value: vs}
				}
				return &object.Hash{Pairs: pairs}
			}
			name, ok := args[0].(*object.String)
			if !ok {
				return newError("physics_const: want string name")
			}
			if v, ok := quantum.GreekLetters[name.Value]; ok {
				return &object.Float{Value: v}
			}
			return newError("physics_const: unknown %s", name.Value)
		},
	}
	builtins["self_eval"] = &object.Builtin{
		Fn: func(args ...object.Object) object.Object {
			// self_eval(src): evaluate NvS source with the full host
			// interpreter (not the mini_eval.ns subset). Errors are
			// returned as NvS error values, never panics.
			if len(args) != 1 {
				return newError("self_eval: want source string")
			}
			src, ok := args[0].(*object.String)
			if !ok {
				return newError("self_eval: want string")
			}
			env := object.NewEnvironment()
			LoadPrelude(env)
			l := lexer.New(src.Value)
			p := parser.New(l)
			program := p.ParseProgram()
			if errs := p.Errors(); len(errs) > 0 {
				return newError("self_eval parse: %s", errs[0])
			}
			return Eval(program, env)
		},
	}
}

// injectPort29PhysicsSymbols sets π/ℏ/c/... globals, mirroring the 2.9 track.
// Greek glyphs the lexer may not accept as identifiers are still reachable
// via physics_const("pi") etc.
func injectPort29PhysicsSymbols(env *object.Environment) {
	putF := func(name string, v float64) {
		env.Set(name, &object.Float{Value: v})
	}
	putF("π", quantum.Pi)
	putF("pi", quantum.Pi)
	putF("τ", quantum.Pi*2)
	putF("tau", quantum.Pi*2)
	putF("φ", (1+math.Sqrt(5))/2)
	putF("phi", (1+math.Sqrt(5))/2)
	putF("ℏ", quantum.Hbar)
	putF("hbar", quantum.Hbar)
	putF("h_planck", quantum.H)
	putF("c_light", quantum.C)
	putF("G_grav", quantum.G)
	putF("k_B", quantum.K_B)
	putF("e_charge", quantum.E_CHARGE)
	putF("m_e", quantum.M_E)
	putF("m_p", quantum.M_P)
	putF("N_A", quantum.NA)
	putF("α", quantum.ALPHA)
	putF("alpha_fs", quantum.ALPHA)
	// Remaining Greek entries default to 0.0; users can reassign them.
	for glyph, val := range quantum.GreekLetters {
		if _, ok := env.Get(glyph); !ok {
			putF(glyph, val)
		}
	}
}

// port29StateToArray converts a state vector to [[re,im], ...].
func port29StateToArray(s quantum.State) *object.Array {
	els := make([]object.Object, len(s))
	for i, a := range s {
		els[i] = &object.Array{Elements: []object.Object{
			&object.Float{Value: real(a)},
			&object.Float{Value: imag(a)},
		}}
	}
	return &object.Array{Elements: els}
}

// port29ArrayToState converts [[re,im], ...] back to a state vector.
func port29ArrayToState(obj object.Object) (quantum.State, error) {
	arr, ok := obj.(*object.Array)
	if !ok {
		return nil, fmt.Errorf("state must be array of [re,im] pairs")
	}
	pairs := make([][2]float64, len(arr.Elements))
	for i, el := range arr.Elements {
		pair, ok := el.(*object.Array)
		if !ok || len(pair.Elements) < 2 {
			return nil, fmt.Errorf("state[%d] must be [re,im]", i)
		}
		re, _ := toFloat(pair.Elements[0])
		im, _ := toFloat(pair.Elements[1])
		pairs[i] = [2]float64{re, im}
	}
	return quantum.FromPairs(pairs), nil
}
