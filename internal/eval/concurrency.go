package eval

// Wave 6 — concurrency robbery: cooperative tasks + channels.
//
// The language-level model is cooperative message-passing (Lua coroutines /
// Erlang processes / JS workers lineage). Under the hood the execution
// mechanism is Go goroutines, but the DOCUMENTED contract — the only thing
// user programs may rely on — is:
//
//   - Tasks switch at explicit yield points only: sleep(), task_yield(), a
//     blocking send()/recv(), or join(). User code can never observe a
//     switch anywhere else.
//   - **NO SHARED MUTABLE STATE.** Values crossing a channel (or passed as
//     spawn arguments) are deep-copied. Channel and task handles pass by
//     reference — they are the communication mechanism, never copied.
//     Functions, builtins, class instances and generators also pass by
//     reference (they cannot be copied). Mutating a value that is visible
//     from two tasks is a data race and a bug in the user program.
//   - The interpreter's Environment store is mutex-guarded, so concurrent
//     top-level `let`/assignment cannot corrupt the map itself. Racy
//     programs get last-writer-wins, and a stern entry in the docs telling
//     them not to do that.
//
// `nvs run` drains all spawned tasks before exiting (DrainSpawnedTasks), so
// no task output is lost to an early exit.

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/navescript/nvs/internal/object"
)

// ---- task registry: lets `nvs run` wait for every spawned task ----

var (
	spawnedMu     sync.Mutex
	spawnedTasks  = make(map[int64]*object.Task)
	taskIDCounter int64
)

func nextTaskID() int64 {
	return atomic.AddInt64(&taskIDCounter, 1)
}

func registerTask(t *object.Task) {
	spawnedMu.Lock()
	spawnedTasks[t.ID] = t
	spawnedMu.Unlock()
}

// DrainSpawnedTasks blocks until every task spawned so far has finished,
// including tasks spawned transitively by other tasks while we wait.
// Called by `nvs run` after the main program completes.
func DrainSpawnedTasks() {
	for {
		spawnedMu.Lock()
		var pending []*object.Task
		for _, t := range spawnedTasks {
			if !t.IsDone() {
				pending = append(pending, t)
			}
		}
		if len(pending) == 0 {
			spawnedTasks = make(map[int64]*object.Task)
			spawnedMu.Unlock()
			return
		}
		spawnedMu.Unlock()
		for _, t := range pending {
			t.Wait()
		}
	}
}

// ---- message copying ----

// deepCopyMessage deep-copies a value crossing a task boundary (spawn
// argument or channel send). Mutable containers — arrays, hashes, tuples,
// records — are copied recursively, preserving frozen-ness, so the sender
// can never observe or disturb what the receiver got. Scalars are
// immutable and shared. Handles pass by reference and are never copied:
// channels, tasks (the communication mechanism), functions, builtins,
// class instances, generators. Errors are copied so a task's error value
// is independent of the task that raised it.
func deepCopyMessage(obj object.Object) object.Object {
	switch o := obj.(type) {
	case *object.Array:
		el := make([]object.Object, len(o.Elements))
		for i, e := range o.Elements {
			el[i] = deepCopyMessage(e)
		}
		return &object.Array{Elements: el, Frozen: o.Frozen}
	case *object.Hash:
		pairs := make(map[object.HashKey]object.HashPair, len(o.Pairs))
		for k, p := range o.Pairs {
			pairs[k] = object.HashPair{Key: deepCopyMessage(p.Key), Value: deepCopyMessage(p.Value)}
		}
		return &object.Hash{Pairs: pairs, Frozen: o.Frozen}
	case *object.Tuple:
		el := make([]object.Object, len(o.Elements))
		for i, e := range o.Elements {
			el[i] = deepCopyMessage(e)
		}
		return &object.Tuple{Elements: el}
	case *object.Record:
		vals := make([]object.Object, len(o.Values))
		for i, v := range o.Values {
			vals[i] = deepCopyMessage(v)
		}
		return &object.Record{Def: o.Def, Values: vals}
	case *object.Error:
		return &object.Error{Message: o.Message, Line: o.Line, Column: o.Column}
	default:
		// Immutable scalars (int, float, string, bool, null) are safe to
		// share. Functions, builtins, channels, tasks, instances,
		// generators, classes pass by reference — documented.
		return o
	}
}

// ---- spawning ----

// spawnTask runs callable concurrently and returns its *object.Task handle
// (or an *object.Error). Arguments are deep-copied, except handles. The
// task's own locals live in a fresh enclosed environment via applyFunction;
// anything it touches through its closure's outer scopes is shared —
// don't do that, use channels.
func spawnTask(callable object.Object, args []object.Object) object.Object {
	switch callable.(type) {
	case *object.Function, *object.Builtin:
	default:
		return newError("spawn: first argument must be a function, got %s", callable.Type())
	}
	for _, a := range args {
		if _, ok := a.(*object.NamedArg); ok {
			return newError("spawn: named arguments are not supported; pass arguments positionally")
		}
	}
	copied := make([]object.Object, len(args))
	for i, a := range args {
		copied[i] = deepCopyMessage(a)
	}
	task := object.NewTask(nextTaskID())
	registerTask(task)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				task.Finish(newError("task #%d panicked: %v", task.ID, r))
			}
		}()
		var res object.Object
		if fn, ok := callable.(*object.Function); ok {
			res = applyFunction(fn, copied)
		} else {
			res = callable.(*object.Builtin).Fn(copied...)
		}
		// applyFunction already unwraps return values; an *object.Error
		// result is the task's raised error, delivered to join().
		task.Finish(res)
	}()
	return task
}

// asChannel extracts a *object.Channel or returns an NvS error value.
func asChannel(what string, obj object.Object) (*object.Channel, object.Object) {
	ch, ok := obj.(*object.Channel)
	if !ok {
		return nil, newError("%s: want channel, got %s", what, obj.Type())
	}
	return ch, nil
}

// asTask extracts a *object.Task or returns an NvS error value.
func asTask(what string, obj object.Object) (*object.Task, object.Object) {
	t, ok := obj.(*object.Task)
	if !ok {
		return nil, newError("%s: want task, got %s", what, obj.Type())
	}
	return t, nil
}

// registerWave6Builtins adds the wave-6 concurrency builtins. Called once
// from initBuiltins() (same pattern as registerWave5Builtins). join is
// registered here too: the 1-argument task form is new, the 2-argument
// string form keeps its historical behavior via joinBuiltin.
func registerWave6Builtins() {
	builtins["spawn"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) < 1 {
			return newError("spawn: want function and optional arguments")
		}
		return spawnTask(args[0], args[1:])
	}}

	builtins["chan"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) > 1 {
			return newError("chan: want 0 arguments or a buffer capacity")
		}
		if len(args) == 0 {
			return object.NewChannel(0)
		}
		n, ok := args[0].(*object.Integer)
		if !ok {
			return newError("chan: capacity must be an integer, got %s", args[0].Type())
		}
		if n.Value < 0 {
			return newError("chan: capacity cannot be negative")
		}
		return object.NewChannel(int(n.Value))
	}}

	builtins["send"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 2 {
			return newError("send: want channel, value")
		}
		ch, errObj := asChannel("send", args[0])
		if errObj != nil {
			return errObj
		}
		if ch.IsClosed() {
			return newError("send on closed channel")
		}
		msg := deepCopyMessage(args[1])
		// Blocking send is the yield point: the task parks here until a
		// receiver arrives (rendezvous if unbuffered). A concurrent close
		// between the check above and the send panics in Go; recover and
		// report it as an NvS error instead of crashing the interpreter.
		var sendErr object.Object
		func() {
			defer func() {
				if recover() != nil {
					sendErr = newError("send on closed channel")
				}
			}()
			ch.Send(msg)
		}()
		if sendErr != nil {
			return sendErr
		}
		return NULL
	}}

	builtins["recv"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return newError("recv: want channel")
		}
		ch, errObj := asChannel("recv", args[0])
		if errObj != nil {
			return errObj
		}
		// Blocking receive is a yield point. Returns null once the
		// channel is closed AND drained.
		v, ok := ch.Recv()
		if !ok {
			return NULL
		}
		return v
	}}

	builtins["try_recv"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return newError("try_recv: want channel")
		}
		ch, errObj := asChannel("try_recv", args[0])
		if errObj != nil {
			return errObj
		}
		v, received := ch.TryRecv()
		if !received {
			return &object.Array{Elements: []object.Object{FALSE, NULL}}
		}
		return &object.Array{Elements: []object.Object{TRUE, v}}
	}}

	builtins["try_send"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 2 {
			return newError("try_send: want channel, value")
		}
		ch, errObj := asChannel("try_send", args[0])
		if errObj != nil {
			return errObj
		}
		if ch.IsClosed() {
			return newError("send on closed channel")
		}
		msg := deepCopyMessage(args[1])
		var sent bool
		var panicked bool
		func() {
			defer func() {
				if recover() != nil {
					panicked = true
				}
			}()
			sent = ch.TrySend(msg)
		}()
		if panicked {
			return newError("send on closed channel")
		}
		if sent {
			return TRUE
		}
		return FALSE
	}}

	builtins["close"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return newError("close: want channel")
		}
		ch, errObj := asChannel("close", args[0])
		if errObj != nil {
			return errObj
		}
		if !ch.Close() {
			return newError("close of closed channel")
		}
		return NULL
	}}

	// Named task_yield (not yield): `yield` is the generator keyword in NvS,
	// so a `yield` builtin could never be called.
	builtins["task_yield"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 0 {
			return newError("task_yield: want 0 arguments")
		}
		// Explicit cooperative yield point: let other tasks run.
		runtime.Gosched()
		return NULL
	}}

	builtins["task_status"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 1 {
			return newError("task_status: want task")
		}
		t, errObj := asTask("task_status", args[0])
		if errObj != nil {
			return errObj
		}
		if t.IsDone() {
			return &object.String{Value: "done"}
		}
		return &object.String{Value: "running"}
	}}

	// join is extended here for the 1-argument task form; the 2-argument
	// string form (join(array, sep)) keeps living in the builtins literal
	// in eval.go. Both dispatch through joinBuiltin.
	builtins["join"] = &object.Builtin{Fn: joinBuiltin}

	builtins["pmap"] = &object.Builtin{Fn: func(args ...object.Object) object.Object {
		if len(args) != 2 {
			return newError("pmap: want function, array")
		}
		switch args[0].(type) {
		case *object.Function, *object.Builtin:
		default:
			return newError("pmap: first argument must be a function, got %s", args[0].Type())
		}
		arr, ok := args[1].(*object.Array)
		if !ok {
			return newError("pmap: second argument must be an array, got %s", args[1].Type())
		}
		// One task per element; results collected in order via join, so
		// pmap preserves order while the work overlaps. A task error
		// re-raises at the join, aborting the map.
		tasks := make([]*object.Task, len(arr.Elements))
		for i, el := range arr.Elements {
			t := spawnTask(args[0], []object.Object{el})
			task, ok := t.(*object.Task)
			if !ok {
				return t // spawn error value
			}
			tasks[i] = task
		}
		out := make([]object.Object, len(tasks))
		for i, t := range tasks {
			res := t.Wait()
			if isError(res) {
				return res
			}
			out[i] = res
		}
		return &object.Array{Elements: out}
	}}
}

// joinBuiltin dispatches: join(task) blocks until the task finishes and
// returns its return value — re-raising its error if it raised — while
// join(array, sep) keeps the historical string-join behavior.
func joinBuiltin(args ...object.Object) object.Object {
	if len(args) == 1 {
		t, errObj := asTask("join", args[0])
		if errObj != nil {
			return errObj
		}
		// Blocking on the task is a yield point. An *object.Error result
		// propagates through the evaluator as a raised error, so
		// try/catch around join works.
		return t.Wait()
	}
	if len(args) != 2 {
		return newError("join: want task, or array and separator")
	}
	// Historical string-join behavior, preserved exactly.
	arr, ok := args[0].(*object.Array)
	if !ok {
		return newError("join: first arg must be array")
	}
	sep := ","
	if s, ok := args[1].(*object.String); ok {
		sep = s.Value
	}
	parts := make([]string, len(arr.Elements))
	for i, e := range arr.Elements {
		parts[i] = e.Inspect()
	}
	return &object.String{Value: strings.Join(parts, sep)}
}
