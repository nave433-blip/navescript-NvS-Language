package eval

// Wave 21 tests: small-integer cache and block-statement fast path.

import (
	"testing"

	"github.com/navescript/nvs/internal/object"
)

func TestWave21IntCacheShared(t *testing.T) {
	a := newInteger(42)
	b := newInteger(42)
	if a != b {
		t.Fatalf("small integers should be shared cache objects")
	}
	if a.Value != 42 {
		t.Fatalf("cached integer has wrong value: %d", a.Value)
	}
}

func TestWave21IntCacheBounds(t *testing.T) {
	for _, v := range []int64{0, 1, 255} {
		if newInteger(v).Value != v {
			t.Fatalf("cache miss for %d", v)
		}
	}
	for _, v := range []int64{-1, 256, 1000000} {
		got := newInteger(v)
		if got.Value != v {
			t.Fatalf("newInteger(%d) = %d", v, got.Value)
		}
		if got == newInteger(v) && v >= 0 && v < 256 {
			t.Fatalf("out-of-cache value %d should not be shared", v)
		}
	}
}

func TestWave21IntCacheLoopCorrectness(t *testing.T) {
	// Heavy small-int traffic: literals, arithmetic, loop counters.
	expectInspect(t, `
let total = 0;
for (let i = 0; i < 1000; i = i + 1) { total = total + i; }
total`, `499500`)
}

func TestWave21BlockReturnStillPropagates(t *testing.T) {
	expectInspect(t, `
fn f() { let x = 1; return x + 1; let y = 999; }
f()`, `2`)
}

func TestWave21BlockErrorStillPropagates(t *testing.T) {
	// An error mid-block must still abort the block.
	res := testEval(t, `fn f() { let a = 1; bogus_fn_xyz(); let b = 2; } f()`)
	if _, ok := res.(*object.Error); !ok {
		t.Fatalf("expected error to propagate, got %v (%T)", res.Inspect(), res)
	}
}

func TestWave21ConstStillEnforced(t *testing.T) {
	// const must work with lazily-allocated constants map.
	expectInspect(t, `const x = 5; x`, `5`)
	res := testEval(t, `const y = 1; y = 2;`)
	if _, ok := res.(*object.Error); !ok {
		t.Fatalf("expected const-violation error, got %v (%T)", res.Inspect(), res)
	}
}

func TestWave21AnnotatedLetStillWorks(t *testing.T) {
	// declaredTypes map is also lazy now.
	expectInspect(t, `let z: int = 41 + 1; z`, `42`)
}
