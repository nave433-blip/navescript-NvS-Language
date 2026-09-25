package eval

import (
	"github.com/navescript/nvs/internal/object"
)

// Wave 10: host-language bridges (C ABI via cbridge, JSON stdio via
// `nvs bridge`) need to drive the evaluator from outside this package.
// This wrapper exposes the otherwise-internal entry point.
//
// (The builtin registry is already exposed as BuiltinNames in
// builtins_list.go; the transpiler uses it to reject unmapped builtins
// instead of emitting calls that could never work.)

// ApplyFunction applies fn (a *object.Function or *object.Builtin) to args.
// It returns the call result, or an *object.Error on failure (unknown
// function type, arity mismatch, annotation violation, runtime error).
func ApplyFunction(fn object.Object, args []object.Object) object.Object {
	return applyFunction(fn, args)
}

// LookupBuiltin returns the builtin registered under name, mirroring how
// evalIdentifier resolves identifiers that are not bound in the
// environment. Bridge sessions use it so nvs_call can reach builtins
// (len, str, ...) as well as user-defined functions.
func LookupBuiltin(name string) (*object.Builtin, bool) {
	ensureBuiltins()
	b, ok := builtins[name]
	return b, ok
}
