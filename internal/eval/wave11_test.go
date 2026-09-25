package eval

import "testing"

// Wave 11: for-in over generators, and the map() builtin (completing the
// map/filter/reduce trio).

func TestForInGenerator(t *testing.T) {
	expectInspect(t,
		`fn gen() { yield 10; yield 20; yield 30 }
		 total = 0
		 for (x in gen()) { total = total + x }
		 total`,
		"60")
}

func TestForInGeneratorPartialConsumption(t *testing.T) {
	// next() consumed the first value; for-in yields only the remainder.
	expectInspect(t,
		`fn gen() { yield 1; yield 2; yield 3 }
		 g = gen()
		 first = next(g)
		 rest = []
		 for (x in g) { rest = push(rest, x) }
		 [first, rest]`,
		"[1, [2, 3]]")
}

func TestForInGeneratorBreak(t *testing.T) {
	expectInspect(t,
		`fn gen() { yield 1; yield 2; yield 3 }
		 seen = []
		 for (x in gen()) { seen = push(seen, x); if (x == 2) { break } }
		 seen`,
		"[1, 2]")
}

func TestForInGeneratorNoYieldsIsNull(t *testing.T) {
	// A function with no yield statements is not a generator (dynamic
	// semantics, unchanged): calling it yields NULL, and for-in over NULL
	// is an honest error.
	expectErrorContains(t,
		`fn gen() { }
		 for (x in gen()) { }`,
		"for-in not supported on NULL")
}

func TestMapBuiltin(t *testing.T) {
	expectInspect(t, `map([1, 2, 3], fn(x) { return x * x })`, "[1, 4, 9]")
	expectInspect(t, `map([], fn(x) { return x })`, "[]")
	expectInspect(t,
		`map(filter([1, 2, 3, 4], fn(x) { return x % 2 == 0 }), fn(x) { return x * 10 })`,
		"[20, 40]")
	expectErrorContains(t, `map(123, fn(x) { return x })`, "map: want array")
	expectErrorContains(t, `map([1])`, "map: want array, function")
}

func TestMapBuiltinErrorPropagation(t *testing.T) {
	expectErrorContains(t,
		`map([1, 2], fn(x) { if (x == 2) { throw "bad" } return x })`,
		"bad")
}
