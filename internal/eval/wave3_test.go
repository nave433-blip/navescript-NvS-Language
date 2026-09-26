package eval

import (
	"strings"
	"testing"

	"github.com/navescript/nvs/internal/lexer"
	"github.com/navescript/nvs/internal/object"
	"github.com/navescript/nvs/internal/parser"
)

func testEval(t *testing.T, input string) object.Object {
	t.Helper()
	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors for %q: %v", input, p.Errors())
	}
	env := object.NewEnvironment()
	return Eval(prog, env)
}

func expectInspect(t *testing.T, input, want string) {
	t.Helper()
	got := testEval(t, input)
	if isError(got) {
		t.Fatalf("eval %q: unexpected error: %s", input, got.Inspect())
	}
	if got.Inspect() != want {
		t.Fatalf("eval %q: got %q, want %q", input, got.Inspect(), want)
	}
}

func expectErrorContains(t *testing.T, input, substr string) {
	t.Helper()
	got := testEval(t, input)
	if !isError(got) {
		t.Fatalf("eval %q: expected error containing %q, got %s", input, substr, got.Inspect())
	}
	if !strings.Contains(got.Inspect(), substr) {
		t.Fatalf("eval %q: error %q does not contain %q", input, got.Inspect(), substr)
	}
}

// ---- Tuples ----

func TestWave3TupleLiteral(t *testing.T) {
	expectInspect(t, `(1, 2, 3)`, `(1, 2, 3)`)
	expectInspect(t, `(1,)`, `(1,)`)
	expectInspect(t, `()`, `()`)
	expectInspect(t, `(1, (2, 3))`, `(1, (2, 3))`)
	expectInspect(t, `("a", 1, true)`, `(a, 1, true)`)
}

func TestWave3TupleIndexSlice(t *testing.T) {
	expectInspect(t, `let t = (10, 20, 30)
t[0]`, `10`)
	expectInspect(t, `let t = (10, 20, 30)
t[2]`, `30`)
	expectInspect(t, `let t = (10, 20, 30)
t[9]`, `null`)
	expectInspect(t, `let t = (10, 20, 30)
t[0:2]`, `(10, 20)`)
	expectInspect(t, `type((1, 2)[0:1])`, `TUPLE`)
}

func TestWave3TupleInLenIter(t *testing.T) {
	expectInspect(t, `2 in (1, 2, 3)`, `true`)
	expectInspect(t, `9 in (1, 2, 3)`, `false`)
	expectInspect(t, `len((1, 2, 3))`, `3`)
	expectInspect(t, `len(())`, `0`)
	expectInspect(t, `let s = 0
for (x in (1, 2, 3)) { s = s + x }
s`, `6`)
}

func TestWave3TupleBuiltin(t *testing.T) {
	expectInspect(t, `tuple([1, 2])`, `(1, 2)`)
	expectInspect(t, `type(tuple([1]))`, `TUPLE`)
	expectErrorContains(t, `tuple(5)`, `must be array`)
	expectErrorContains(t, `tuple()`, `want 1 argument`)
}

func TestWave3TupleImmutable(t *testing.T) {
	expectErrorContains(t, `let t = (1, 2)
t[0] = 9`, `immutable tuple`)
	expectErrorContains(t, `let t = (1, 2)
push(t, 3)`, `must be array`)
	expectErrorContains(t, `let t = (1, 2)
pop(t)`, `want array`)
}

func TestWave3TupleEquality(t *testing.T) {
	expectInspect(t, `(1, 2) == (1, 2)`, `true`)
	expectInspect(t, `(1, 2) == (1, 3)`, `false`)
	expectInspect(t, `(1, 2) != (1, 2)`, `false`)
	expectInspect(t, `() == ()`, `true`)
	expectInspect(t, `deep_equal((1, (2, 3)), (1, (2, 3)))`, `true`)
}

// ---- Records ----

func TestWave3RecordConstruct(t *testing.T) {
	expectInspect(t, `record Point(x, y)
Point(1, 2)`, `Point(x=1, y=2)`)
	expectInspect(t, `record Point(x, y)
Point(x: 1, y: 2)`, `Point(x=1, y=2)`)
	expectInspect(t, `record Point(x, y)
Point(y: 2, x: 1)`, `Point(x=1, y=2)`)
	expectInspect(t, `record Point(x, y)
Point(1, y: 2)`, `Point(x=1, y=2)`)
	expectInspect(t, `record U()
U()`, `U()`)
}

func TestWave3RecordConstructErrors(t *testing.T) {
	expectErrorContains(t, `record Point(x, y)
Point(1)`, `missing value for field: y`)
	expectErrorContains(t, `record Point(x, y)
Point(1, 2, 3)`, `expects 2 fields`)
	expectErrorContains(t, `record Point(x, y)
Point(z: 1, x: 1, y: 2)`, `no field: z`)
	expectErrorContains(t, `record Point(x, y)
Point(1, x: 5, y: 2)`, `duplicate value for field: x`)
}

func TestWave3RecordFieldAccess(t *testing.T) {
	expectInspect(t, `record Point(x, y)
let p = Point(3, 4)
p.x`, `3`)
	expectInspect(t, `record Point(x, y)
let p = Point(3, 4)
p.y`, `4`)
	expectInspect(t, `record Point(x, y)
let p = Point(3, 4)
p[1]`, `4`)
	expectInspect(t, `record Point(x, y)
let p = Point(3, 4)
p?.x`, `3`)
	expectErrorContains(t, `record Point(x, y)
Point(1, 2).z`, `no field: z`)
}

func TestWave3RecordEquality(t *testing.T) {
	expectInspect(t, `record P(x, y)
P(1, 2) == P(1, 2)`, `true`)
	expectInspect(t, `record P(x, y)
P(1, 2) == P(1, 3)`, `false`)
	expectInspect(t, `record P(x, y)
P(1, 2) != P(1, 2)`, `false`)
	expectInspect(t, `record P(x)
record Q(x)
P(1) == Q(1)`, `false`)
	expectInspect(t, `record P(x, y)
deep_equal(P(1, [2]), P(1, [2]))`, `true`)
}

func TestWave3RecordImmutable(t *testing.T) {
	expectErrorContains(t, `record P(x)
let p = P(1)
p.x = 2`, `immutable record`)
	expectErrorContains(t, `record P(x)
let p = P(1)
p[0] = 2`, `immutable record`)
}

func TestWave3RecordDestructure(t *testing.T) {
	expectInspect(t, `record Point(x, y)
let {x, y} = Point(3, 4)
x + y`, `7`)
	expectInspect(t, `record Point(x, y)
let [a, b] = Point(3, 4)
a * b`, `12`)
	expectInspect(t, `let [a, b] = (5, 6)
a + b`, `11`)
	expectInspect(t, `record Point(x, y)
let s = 0
for (v in Point(1, 2)) { s = s + v }
s`, `3`)
}

// ---- Freeze / thaw ----

func TestWave3FreezeBasics(t *testing.T) {
	expectInspect(t, `is_frozen(freeze([1, 2]))`, `true`)
	expectInspect(t, `is_frozen([1, 2])`, `false`)
	expectInspect(t, `is_frozen(freeze({"a": 1}))`, `true`)
	expectInspect(t, `is_frozen((1, 2))`, `true`)
	expectInspect(t, `is_frozen(42)`, `true`)
	// Deep: nested values are frozen too.
	expectInspect(t, `let f = freeze({"a": [1]})
is_frozen(f["a"])`, `true`)
}

func TestWave3FreezeRejectsMutation(t *testing.T) {
	expectErrorContains(t, `push(freeze([1]), 2)`, `frozen array`)
	expectErrorContains(t, `pop(freeze([1]))`, `frozen array`)
	expectErrorContains(t, `sort(freeze([2, 1]))`, `frozen array`)
	expectErrorContains(t, `reverse(freeze([1]))`, `frozen array`)
	expectErrorContains(t, `let f = freeze([1])
f[0] = 9`, `frozen array`)
	expectErrorContains(t, `let f = freeze({"a": 1})
f["a"] = 9`, `frozen hash`)
	expectErrorContains(t, `let f = freeze({"a": 1})
f.a = 9`, `frozen hash`)
	expectErrorContains(t, `delete(freeze({"a": 1}), "a")`, `frozen hash`)
	expectErrorContains(t, `set_add(freeze(set(1)), 2)`, `frozen set`)
	// Nested mutation is rejected as well.
	expectErrorContains(t, `let f = freeze({"a": [1]})
f["a"][0] = 9`, `frozen array`)
}

func TestWave3FreezeErrors(t *testing.T) {
	expectInspect(t, `freeze(41 + 1)`, `42`) // scalars pass through
	expectErrorContains(t, `freeze(len)`, `cannot freeze BUILTIN`)
}

func TestWave3Thaw(t *testing.T) {
	expectInspect(t, `let f = freeze({"a": [1, 2]})
let m = thaw(f)
m["a"][0] = 9
m["a"][0]`, `9`)
	expectInspect(t, `let f = freeze({"a": [1]})
let m = thaw(f)
is_frozen(m)`, `false`)
	expectInspect(t, `let f = freeze({"a": [1]})
let m = thaw(f)
is_frozen(m["a"])`, `false`)
	// Original stays frozen.
	expectErrorContains(t, `let f = freeze([1])
let m = thaw(f)
push(m, 2)
push(f, 3)`, `frozen array`)
	// thaw(tuple) -> mutable array; thaw(record) -> hash of fields.
	expectInspect(t, `let m = thaw((1, 2))
push(m, 3)
m`, `[1, 2, 3]`)
	expectInspect(t, `record P(x, y)
let m = thaw(P(1, 2))
m["x"] = 9
m["x"]`, `9`)
}

func TestWave3FreezeCycleSafe(t *testing.T) {
	// Cyclic structures must not hang freeze/thaw.
	expectInspect(t, `let a = []
push(a, a)
freeze(a)
is_frozen(a)`, `true`)
	expectInspect(t, `let a = []
push(a, a)
let b = thaw(a)
len(b)`, `1`)
}

// ---- deep_get / deep_set ----

func TestWave3DeepGet(t *testing.T) {
	expectInspect(t, `deep_get({"a": {"b": 42}}, "a.b")`, `42`)
	expectInspect(t, `deep_get({"a": [{"b": 1}]}, "a.0.b")`, `1`)
	expectInspect(t, `deep_get({"a": 1}, "a.b.c")`, `null`)
	expectInspect(t, `deep_get({"a": 1}, "a.b.c", "dflt")`, `dflt`)
	expectInspect(t, `deep_get({"a": [1, 2]}, "a.5", 0)`, `0`)
	expectInspect(t, `record P(x)
deep_get({"p": P(7)}, "p.x")`, `7`)
	expectInspect(t, `deep_get((1, (2, 3)), "1.0")`, `2`)
	expectInspect(t, `deep_get(null, "a.b", 9)`, `9`)
	expectErrorContains(t, `deep_get({"a": 1}, "a..b")`, `empty path segment`)
	expectErrorContains(t, `deep_get({"a": 1}, 5)`, `path must be string`)
}

func TestWave3DeepSet(t *testing.T) {
	expectInspect(t, `let m = {"a": {"b": 1}}
deep_set(m, "a.b", 2)
m["a"]["b"]`, `2`)
	expectInspect(t, `let m = {}
deep_set(m, "a.b.c", 1)
m["a"]["b"]["c"]`, `1`)
	expectInspect(t, `let m = {"a": [10, 20]}
deep_set(m, "a.1", 99)
m["a"][1]`, `99`)
	expectInspect(t, `let m = {"a": 1}
deep_set(m, "a", 2)
m`, `{a: 2}`)
	// Root is returned for chaining.
	expectInspect(t, `deep_get(deep_set({}, "a.b", 5), "a.b")`, `5`)
	expectErrorContains(t, `let m = {"a": [1]}
deep_set(m, "a.5", 2)`, `out of range`)
	expectErrorContains(t, `let m = {"a": 1}
deep_set(m, "a.b", 2)`, `cannot set field "b" on INTEGER`)
	expectErrorContains(t, `deep_set((1, 2), "0", 9)`, `immutable tuple`)
	expectErrorContains(t, `record P(x)
deep_set(P(1), "x", 2)`, `immutable record`)
	expectErrorContains(t, `let f = freeze({"a": 1})
deep_set(f, "a", 2)`, `frozen hash`)
	expectErrorContains(t, `let f = freeze([1])
deep_set(f, "0", 2)`, `frozen array`)
}

// ---- Set / map builtins ----

func TestWave3SetOps(t *testing.T) {
	expectInspect(t, `set_len(set_union(set(1, 2), set(2, 3)))`, `3`)
	expectInspect(t, `set_to_array(set_intersect(set(1, 2), set(2, 3)))`, `[2]`)
	expectInspect(t, `set_to_array(set_diff(set(1, 2, 3), set(2)))`, `[1, 3]`)
	expectInspect(t, `set_len(set(1, 2, 3))`, `3`)
	expectInspect(t, `len(set(1, 2))`, `2`)
	expectInspect(t, `let s = set(1)
set_add(s, 2)
set_has(s, 2)`, `true`)
	expectInspect(t, `let s = set(1, 2)
set_remove(s, 1)
set_has(s, 1)`, `false`)
	expectErrorContains(t, `set_union(set(1), [2])`, `must be sets`)
}

func TestWave3MapBuiltins(t *testing.T) {
	expectInspect(t, `map_merge({"a": 1}, {"a": 9, "b": 2})["a"]`, `9`)
	expectInspect(t, `len(map_merge({"a": 1}, {"b": 2}))`, `2`)
	expectInspect(t, `len(map_pick({"a": 1, "b": 2}, "a"))`, `1`)
	expectInspect(t, `len(map_pick({"a": 1, "b": 2}, ["a", "b"]))`, `2`)
	expectInspect(t, `len(map_omit({"a": 1, "b": 2}, "a"))`, `1`)
	expectInspect(t, `len(map_omit({"a": 1, "b": 2}, ["a", "b"]))`, `0`)
	expectInspect(t, `invert({"a": 1})[1]`, `a`)
	expectInspect(t, `len({"a": 1, "b": 2})`, `2`)
	expectInspect(t, `len((1, 2, 3))`, `3`)
	expectInspect(t, `record P(x, y)
len(P(1, 2))`, `2`)
	expectErrorContains(t, `map_merge({"a": 1}, [2])`, `must be maps`)
	expectErrorContains(t, `map_pick({"a": 1}, 5)`, `keys must be strings`)
	expectErrorContains(t, `invert({"a": [1]})`, `not hashable`)
}
