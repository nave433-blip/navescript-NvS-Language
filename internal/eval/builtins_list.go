package eval

import "sort"

// BuiltinNames returns the sorted names of all registered Go builtins
// (len, print, ...). It exists so the `nvs lint` tool can implement its
// shadow-builtin rule against the real registry instead of a hardcoded
// copy that would drift.
//
// Honest scope: this covers only Go-registered builtins. Functions defined
// in stdlib/prelude.ns are NOT included.
func BuiltinNames() []string {
	ensureBuiltins()
	names := make([]string, 0, len(builtins))
	for n := range builtins {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
