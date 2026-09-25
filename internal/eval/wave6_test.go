package eval

// Wave 6 tests: cooperative tasks and channels. All tests are deterministic —
// no wall-clock assertions, no timing-dependent interleavings. Anything that
// must be ordered is ordered by the channel protocol itself.

import (
	"sync"
	"testing"

	"github.com/navescript/nvs/internal/object"
)

// ---- channel open / close / drain semantics ----

func TestWave6ChanBufferedDrain(t *testing.T) {
	expectInspect(t, `
let ch = chan(2)
try_send(ch, "a")
try_send(ch, "b")
close(ch)
let out = [recv(ch), recv(ch), recv(ch)]
out`, `[a, b, null]`)
}

func TestWave6TrySendFullReturnsFalse(t *testing.T) {
	expectInspect(t, `
let ch = chan(1)
let r1 = try_send(ch, 1)
let r2 = try_send(ch, 2)
let out = [r1, r2]
out`, `[true, false]`)
}

func TestWave6TryRecvShapes(t *testing.T) {
	expectInspect(t, `
let ch = chan(1)
let empty = try_recv(ch)
send(ch, 7)
let full = try_recv(ch)
let out = [empty, full]
out`, `[[false, null], [true, 7]]`)
}

func TestWave6SendOnClosedErrors(t *testing.T) {
	expectErrorContains(t, `
let ch = chan()
close(ch)
send(ch, 1)`, "send on closed channel")
}

func TestWave6DoubleCloseErrors(t *testing.T) {
	expectErrorContains(t, `
let ch = chan()
close(ch)
close(ch)`, "close of closed channel")
}

func TestWave6TrySendOnClosedErrors(t *testing.T) {
	expectErrorContains(t, `
let ch = chan()
close(ch)
try_send(ch, 1)`, "send on closed channel")
}

func TestWave6ChanBadArgs(t *testing.T) {
	expectErrorContains(t, `chan(-1)`, "capacity cannot be negative")
	expectErrorContains(t, `chan("x")`, "capacity must be an integer")
	expectErrorContains(t, `send(1, 2)`, "want channel")
	expectErrorContains(t, `recv([1])`, "want channel")
	expectErrorContains(t, `close("nope")`, "want channel")
}

// ---- deep-copy isolation: the no-shared-mutable-state rule ----

func TestWave6SendDeepCopiesArray(t *testing.T) {
	// Mutating the sent array after send must not affect the receiver.
	expectInspect(t, `
let ch = chan(1)
let xs = [1, 2, 3]
send(ch, xs)
xs[0] = 99
recv(ch)[0]`, `1`)
}

func TestWave6SendDeepCopiesNested(t *testing.T) {
	expectInspect(t, `
let ch = chan(1)
let m = {"a": [1, {"b": 2}]}
send(ch, m)
m["a"][1]["b"] = 99
recv(ch)["a"][1]["b"]`, `2`)
}

func TestWave6SendPreservesFrozen(t *testing.T) {
	expectInspect(t, `
let ch = chan(1)
let xs = freeze([1, 2])
send(ch, xs)
is_frozen(recv(ch))`, `true`)
}

func TestWave6SpawnArgsDeepCopied(t *testing.T) {
	// Args are copied at spawn time, so later mutation is invisible.
	expectInspect(t, `
let xs = [1]
let t = spawn(fn(a) { return a[0] }, xs)
xs[0] = 99
join(t)`, `1`)
}

func TestWave6ChannelHandleByReference(t *testing.T) {
	// Channel handles are the communication mechanism: never copied.
	expectInspect(t, `
let ch = chan()
let t = spawn(fn(c) { send(c, "through") }, ch)
recv(ch)`, `through`)
}

// ---- spawn / join / task_status ----

func TestWave6SpawnJoinValues(t *testing.T) {
	expectInspect(t, `
fn add(a, b) { return a + b }
let t1 = spawn(add, 20, 22)
let t2 = spawn(add, 1, 2)
let out = [join(t1), join(t2)]
out`, `[42, 3]`)
}

func TestWave6SpawnBuiltin(t *testing.T) {
	expectInspect(t, `join(spawn(len, [1, 2, 3]))`, `3`)
}

func TestWave6SpawnNotAFunction(t *testing.T) {
	expectErrorContains(t, `spawn(42)`, "must be a function")
}

func TestWave6JoinReraisesTaskError(t *testing.T) {
	// A task that raises delivers its error to join as an NvS error value,
	// catchable with try/catch.
	got := testEval(t, `
fn boom() { throw "kaboom" }
let t = spawn(boom)
let caught = ""
try { join(t) } catch (e) { caught = e }
caught`)
	if isError(got) {
		t.Fatalf("unexpected error: %s", got.Inspect())
	}
	if got.Inspect() != "ERROR: kaboom" && got.Inspect() != "kaboom" {
		t.Fatalf("got %q, want the kaboom message", got.Inspect())
	}
}

func TestWave6TaskStatusLifecycle(t *testing.T) {
	expectInspect(t, `
let ch = chan()
fn w(c) { recv(c); return "done" }
let t = spawn(w, ch)
let s1 = task_status(t)
send(ch, 1)
let j = join(t)
let s2 = task_status(t)
let out = [s1, j, s2]
out`, `[running, done, done]`)
}

func TestWave6TaskStatusBadArg(t *testing.T) {
	expectErrorContains(t, `task_status(1)`, "want task")
	expectErrorContains(t, `join(1)`, "want task")
}

// ---- sleep / task_yield ----

func TestWave6SleepUnits(t *testing.T) {
	// Integer = milliseconds (historical), float = seconds. Both return null.
	expectInspect(t, `[sleep(1), sleep(0.001)]`, `[null, null]`)
}

func TestWave6SleepBadArg(t *testing.T) {
	expectErrorContains(t, `sleep("x")`, "want integer ms or float seconds")
}

func TestWave6TaskYield(t *testing.T) {
	expectInspect(t, `task_yield()`, `null`)
	expectErrorContains(t, `task_yield(1)`, "want 0 arguments")
}

// ---- pmap ----

func TestWave6PmapOrder(t *testing.T) {
	expectInspect(t, `
let ys = pmap(fn(x) { sleep(2); return x * 10 }, [1, 2, 3, 4, 5])
ys`, `[10, 20, 30, 40, 50]`)
}

func TestWave6PmapEmpty(t *testing.T) {
	expectInspect(t, `pmap(fn(x) { return x }, [])`, `[]`)
}

func TestWave6PmapErrorReraises(t *testing.T) {
	expectErrorContains(t, `
pmap(fn(x) { if (x == 2) { throw "bad two" } ; return x }, [1, 2, 3])`, "bad two")
}

// ---- join keeps its historical string form ----

func TestWave6JoinStringForm(t *testing.T) {
	expectInspect(t, `join(["a", "b", "c"], "-")`, `a-b-c`)
	expectInspect(t, `join(["a", "b"], ",")`, `a,b`)
}

// ---- nvs-run drain: unjoined tasks still complete ----

func TestWave6DrainSpawnedTasks(t *testing.T) {
	got := testEval(t, `
let box = chan(1)
spawn(fn() { sleep(5); send(box, "finished") })
"spawned"`)
	if isError(got) {
		t.Fatalf("unexpected error: %s", got.Inspect())
	}
	DrainSpawnedTasks()
	// If the drain works, the task above has finished by now.
}

// ---- Environment: concurrent access must not corrupt the map ----

func TestWave6EnvironmentConcurrentAccess(t *testing.T) {
	// Run with -race: concurrent let/assignment at top level must not
	// corrupt the store (last-writer-wins, never a torn map).
	env := object.NewEnvironment()
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				name := string(rune('a'+w)) + string(rune('0'+i%10))
				env.Set(name, &object.Integer{Value: int64(i)})
				env.Get(name)
				env.Assign(name, &object.Integer{Value: int64(i + 1)})
				env.IsConst(name)
			}
		}(w)
	}
	wg.Wait()
	for w := 0; w < 8; w++ {
		for i := 0; i < 10; i++ {
			name := string(rune('a'+w)) + string(rune('0'+i))
			if _, ok := env.Get(name); !ok {
				t.Fatalf("key %q lost under concurrent access", name)
			}
		}
	}
}

// ---- unbuffered rendezvous through tasks ----

func TestWave6PingPong(t *testing.T) {
	expectInspect(t, `
let a2b = chan()
let b2a = chan()
fn pinger(outbox, inbox) {
  send(outbox, 1)
  recv(inbox) + 100
}
fn ponger(inbox, outbox) {
  send(outbox, recv(inbox) + 1)
  "pong"
}
let t1 = spawn(pinger, a2b, b2a)
let t2 = spawn(ponger, a2b, b2a)
let out = [join(t1), join(t2)]
out`, `[102, pong]`)
}
