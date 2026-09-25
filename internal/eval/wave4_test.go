package eval

import (
	"testing"
)

// ---- Wave 4: pattern matching ----

func TestWave4GuardBasic(t *testing.T) {
	expectInspect(t, `match (5) { case x if x > 0: { "pos" } case x if x < 0: { "neg" } default: { "zero" } }`, "pos")
	expectInspect(t, `match (-5) { case x if x > 0: { "pos" } case x if x < 0: { "neg" } default: { "zero" } }`, "neg")
	expectInspect(t, `match (0) { case x if x > 0: { "pos" } case x if x < 0: { "neg" } default: { "zero" } }`, "zero")
}

func TestWave4GuardFallThrough(t *testing.T) {
	// A failing guard is not an error: matching continues at the next arm.
	expectInspect(t, `match (5) { case x if x > 100: { "big" } case x: { "small" } }`, "small")
	expectInspect(t, `match ([1, 2]) { case [a, b] if a != b: { "diff" } case [a, b]: { "same" } }`, "diff")
	expectInspect(t, `match ([2, 2]) { case [a, b] if a != b: { "diff" } case [a, b]: { "same" } }`, "same")
}

func TestWave4GuardSeesBindings(t *testing.T) {
	expectInspect(t, `match ({"x": 3}) { case {x} if x > 2: { "big-x" } default: { "other" } }`, "big-x")
	expectInspect(t, `match ({"x": 1}) { case {x} if x > 2: { "big-x" } default: { "other" } }`, "other")
}

func TestWave4GuardNoLeakOnFailure(t *testing.T) {
	// Bindings from a guard-failed arm must not be visible afterwards.
	expectErrorContains(t, `match (5) { case zzz if zzz > 100: { 1 } default: { 2 } }
zzz`, "identifier not found: zzz")
}

func TestWave4GuardErrorPropagates(t *testing.T) {
	expectErrorContains(t, `match (5) { case x if yyy: { 1 } default: { 2 } }`, "identifier not found: yyy")
}

func TestWave4BareIdentifierBinds(t *testing.T) {
	// Previously `case x:` on an undefined name was a runtime error; now it
	// binds (and `_` is a true wildcard).
	expectInspect(t, `match (42) { case v: { v + 1 } }`, "43")
	expectInspect(t, `match (42) { case 1: { "one" } case _: { "wild" } }`, "wild")
}

func TestWave4NoBindingLeak(t *testing.T) {
	// Successful matches bind into the arm's scope only, not the enclosing
	// environment (no example depended on the old leak).
	expectErrorContains(t, `match (1) { case qqq: { qqq } }
qqq`, "identifier not found: qqq")
}

func TestWave4ArrayPattern(t *testing.T) {
	expectInspect(t, `match ([1, 2]) { case [a, b]: { a + b } default: { 0 } }`, "3")
	// refutable: wrong length falls through
	expectInspect(t, `match ([1, 2, 3]) { case [a, b]: { "two" } default: { "other" } }`, "other")
	// non-sequence falls through
	expectInspect(t, `match (7) { case [a, b]: { "seq" } default: { "other" } }`, "other")
	// literal elements test by value
	expectInspect(t, `match ([1, 9]) { case [1, x]: { x } default: { 0 } }`, "9")
	expectInspect(t, `match ([2, 9]) { case [1, x]: { x } default: { 0 } }`, "0")
	// wildcard element
	expectInspect(t, `match ([1, 2]) { case [_, b]: { b } default: { 0 } }`, "2")
	// tuples destructure positionally too
	expectInspect(t, `match ((4, 5)) { case [a, b]: { a * b } default: { 0 } }`, "20")
}

func TestWave4ArrayRestPattern(t *testing.T) {
	expectInspect(t, `match ([1, 2, 3]) { case [h, ...t]: { h + len(t) } default: { 0 } }`, "3")
	expectInspect(t, `match ([1]) { case [h, ...t]: { len(t) } default: { 9 } }`, "0")
	expectInspect(t, `match ([]) { case [h, ...t]: { 1 } default: { 9 } }`, "9")
}

func TestWave4TuplePattern(t *testing.T) {
	expectInspect(t, `match ((10, 20)) { case (a, b): { a + b } default: { 0 } }`, "30")
	expectInspect(t, `match ((10, 20, 30)) { case (a, b): { "two" } default: { "other" } }`, "other")
	expectInspect(t, `match ([10, 20]) { case (a, b): { a + b } default: { 0 } }`, "30")
}

func TestWave4HashPattern(t *testing.T) {
	expectInspect(t, `match ({"x": 1, "y": 2}) { case {x, y}: { x + y } default: { 0 } }`, "3")
	expectInspect(t, `match ({"x": 1}) { case {x, y}: { "both" } case {x}: { x * 10 } default: { 0 } }`, "10")
	// refutable: missing key falls through (unlike let destructuring)
	expectInspect(t, `match ({"y": 1}) { case {x}: { 1 } default: { 2 } }`, "2")
	// refutable: non-hash falls through
	expectInspect(t, `match (42) { case {x}: { 1 } default: { 2 } }`, "2")
	// renamed binding
	expectInspect(t, `match ({"k": 5}) { case {k: v}: { v * 2 } default: { 0 } }`, "10")
	// hash pattern against a record: by field name
	expectInspect(t, `record Pt(x, y)
match (Pt(3, 4)) { case {x, y}: { x + y } default: { 0 } }`, "7")
}

func TestWave4HashLiteralPattern(t *testing.T) {
	// Literal hash patterns keep comparing by value.
	expectInspect(t, `match ({"x": 1}) { case {"x": 1}: { "yes" } default: { "no" } }`, "yes")
	expectInspect(t, `match ({"x": 2}) { case {"x": 1}: { "yes" } default: { "no" } }`, "no")
	// identifier keys in patterns are field names; literal values test.
	expectInspect(t, `match ({"x": 1}) { case {x: 1}: { "yes" } default: { "no" } }`, "yes")
}

func TestWave4RecordPattern(t *testing.T) {
	expectInspect(t, `record Point(x, y)
match (Point(3, 4)) { case Point(a, b): { a + b } default: { 0 } }`, "7")
	// refutable: arity mismatch falls through
	expectInspect(t, `record Point(x, y)
match (Point(3, 4)) { case Point(a): { 1 } default: { 2 } }`, "2")
	// refutable: non-record falls through
	expectInspect(t, `record Point(x, y)
match ([3, 4]) { case Point(a, b): { 1 } default: { 2 } }`, "2")
	// refutable: wrong record type falls through
	expectInspect(t, `record Point(x, y)
record Other(x, y)
match (Other(1, 2)) { case Point(a, b): { 1 } default: { 2 } }`, "2")
	// named-field form, _ skips a field
	expectInspect(t, `record Point(x, y)
match (Point(3, 4)) { case Point(y: q, x: _): { q } default: { 0 } }`, "4")
	// non-pattern call keeps legacy meaning: construct and compare
	expectInspect(t, `record Point(x, y)
match (Point(3, 4)) { case Point(3, 4): { "eq" } default: { "ne" } }`, "eq")
	// literal field tests inside record patterns
	expectInspect(t, `record Point(x, y)
match (Point(0, 5)) { case Point(0, y): { y } default: { 0 } }`, "5")
	expectInspect(t, `record Point(x, y)
match (Point(1, 5)) { case Point(0, y): { y } default: { 0 } }`, "0")
}

func TestWave4NestedPatterns(t *testing.T) {
	expectInspect(t, `match ([1, [2, 3]]) { case [a, [b, c]]: { a + b + c } default: { 0 } }`, "6")
	expectInspect(t, `record P(x, y)
match ([P(1, 2), P(3, 4)]) { case [P(a, b), P(c, d)]: { a + b + c + d } default: { 0 } }`, "10")
	expectInspect(t, `match ({"pt": [7, 8]}) { case {"pt": [u, v]}: { u * v } default: { 0 } }`, "56")
}

func TestWave4MatchAsExpression(t *testing.T) {
	expectInspect(t, `let r = match (2) { case 1: "one"; default: "other" }
r`, "other")
	expectInspect(t, `let r = match (1) { case 1: "one"; default: "other" }
r`, "one")
	expectInspect(t, `match (9) { case 1: { "one" } }`, "null")
	expectInspect(t, `let r = match ([5, 5]) { case [a, b] if a == b: "pair"; default: "no" }
r`, "pair")
}

func TestWave4OptionalParens(t *testing.T) {
	expectInspect(t, `let v = 2
match v { case 1: { "one" } case 2: { "two" } default: { "other" } }`, "two")
	expectInspect(t, `match 1 + 1 { case 2: { "two" } default: { "?" } }`, "two")
}

func TestWave4SwitchAlias(t *testing.T) {
	expectInspect(t, `switch (7) { case n if n > 10: { "big" } case n: { "small" } }`, "small")
	expectInspect(t, `switch ([1, 2]) { case [a, b]: { a + b } default: { 0 } }`, "3")
}

func TestWave4LiteralPatternsUnchanged(t *testing.T) {
	expectInspect(t, `match (1) { case 1: { "one" } default: { "other" } }`, "one")
	expectInspect(t, `match ("s") { case "s": { "yes" } default: { "no" } }`, "yes")
	expectInspect(t, `match (null) { case null: { "nil" } default: { "no" } }`, "nil")
	expectInspect(t, `match (true) { case true: { "T" } case false: { "F" } }`, "T")
	expectInspect(t, `match (2.5) { case 2.5: { "f" } default: { "?" } }`, "f")
}
