/*
NvS C ABI bridge — "write NvS once, use it from any language".

Build:

	go build -buildmode=c-shared -o libnvs.so ./cbridge

This produces libnvs.so plus a generated libnvs.h header declaring:

	char* nvs_eval(const char* src);
	char* nvs_call(const char* func_name, const char* args_json);
	void  nvs_free(char* s);

Threading / lifetime contract (also documented in LANGUAGE.md):

  - One global interpreter. All calls are serialized on a mutex; there is
    exactly one persistent NvS environment per process.
  - nvs_eval evaluates a program string in that environment. Functions and
    bindings defined by one call are visible to later calls.
  - nvs_call applies a named NvS function (or builtin) from the persistent
    environment to a JSON array of arguments. Arguments are positional only.
  - Both return a freshly allocated JSON envelope string:

      {"ok":true,"result":<json>}    on success
      {"ok":false,"error":"..."}     on failure

    <json> covers NvS int, float, string, bool, null, array, and hash
    (hash keys must be string/int/bool). Any other NvS value (function,
    class instance, channel, ...) yields ok:false naming the type — values
    are never silently mis-converted.
  - The caller MUST pass every returned string to nvs_free exactly once.
    The input strings (src, func_name, args_json) are borrowed: the bridge
    copies what it needs before returning, so the caller keeps ownership.
  - A non-terminating NvS program blocks the calling thread (same as
    `nvs run`). NvS `print` output goes to the host process's stdout.

Anything with a C FFI can load this: Python (ctypes/cffi), Ruby
(fiddle/ffi), Node (ffi-napi), Rust (extern "C"), C# (P/Invoke), Java
(JNA), LuaJIT, etc. See examples/wave10_ctypes.py for a working Python
client. LANGUAGE.md sketches the other languages, but only the Python
ctypes path and the JSON bridge have live test coverage — treat the rest
as untested sketches until someone runs them.
*/
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	"github.com/navescript/nvs/internal/bridge"
	"github.com/navescript/nvs/internal/polyglot"
)

// session is the one global interpreter for this process.
var session = bridge.NewSession()

//export nvs_eval
func nvs_eval(src *C.char) *C.char {
	if src == nil {
		return C.CString(`{"ok":false,"error":"nvs_eval: null source pointer"}`)
	}
	obj, err := session.Eval(C.GoString(src))
	return C.CString(session.Envelope(obj, err))
}

//export nvs_call
func nvs_call(funcName *C.char, argsJSON *C.char) *C.char {
	if funcName == nil {
		return C.CString(`{"ok":false,"error":"nvs_call: null function name pointer"}`)
	}
	if argsJSON == nil {
		return C.CString(`{"ok":false,"error":"nvs_call: null args pointer"}`)
	}
	args, err := polyglot.DecodeJSONArgs([]byte(C.GoString(argsJSON)))
	if err != nil {
		return C.CString(session.Envelope(nil, err))
	}
	obj, err := session.Call(C.GoString(funcName), args)
	return C.CString(session.Envelope(obj, err))
}

//export nvs_free
func nvs_free(s *C.char) {
	C.free(unsafe.Pointer(s))
}

func main() {}
