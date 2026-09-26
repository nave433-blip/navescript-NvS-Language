//! Safe Rust bindings for the NvS (Navescript) interpreter.
//!
//! NvS exposes a tiny C ABI (see `examples/c_embed/nvs.h`):
//!
//! ```c
//! char *nvs_eval(const char *src);                    // run source
//! char *nvs_call(const char *func, const char *args); // call fn with JSON args
//! void  nvs_free(char *s);                            // free returned strings
//! ```
//!
//! Both calls return a JSON envelope `{"ok":true,"result":<json>}` or
//! `{"ok":false,"error":"..."}`. There is one global interpreter per
//! process (the Go side serializes on a mutex); this crate adds its own
//! mutex so concurrent Rust callers can't interleave requests either.
//!
//! # Example
//!
//! ```no_run
//! let v = nvs::eval("6 * 7").unwrap();
//! assert_eq!(v, serde_json::json!(42));
//! nvs::eval("fn add(a, b) { a + b }").unwrap();
//! let v = nvs::call("add", &serde_json::json!([20, 22])).unwrap();
//! assert_eq!(v, serde_json::json!(42));
//! ```

use std::ffi::{CStr, CString, NulError};
use std::os::raw::c_char;
use std::sync::Mutex;

use serde_json::Value;

// ---------------------------------------------------------------------------
// Raw C ABI
// ---------------------------------------------------------------------------

unsafe extern "C" {
    fn nvs_eval(src: *const c_char) -> *mut c_char;
    fn nvs_call(func_name: *const c_char, args_json: *const c_char) -> *mut c_char;
    fn nvs_free(s: *mut c_char);
}

// One global interpreter: serialize Rust-side too.
static NVS_LOCK: Mutex<()> = Mutex::new(());

/// Take ownership of a C string returned by the bridge, read it, free it.
unsafe fn take_envelope(ptr: *mut c_char) -> Result<String, NvsError> {
    if ptr.is_null() {
        return Err(NvsError::Bridge("nvs returned null pointer".into()));
    }
    // SAFETY: the bridge contract guarantees a valid NUL-terminated string
    // that we own and must free exactly once.
    let s = unsafe { CStr::from_ptr(ptr).to_string_lossy().into_owned() };
    unsafe { nvs_free(ptr) };
    Ok(s)
}

fn decode_envelope(raw: &str) -> Result<Value, NvsError> {
    let env: Value =
        serde_json::from_str(raw).map_err(|e| NvsError::Bridge(format!("bad envelope JSON: {e}")))?;
    if env.get("ok").and_then(Value::as_bool).unwrap_or(false) {
        Ok(env.get("result").cloned().unwrap_or(Value::Null))
    } else {
        let msg = env
            .get("error")
            .and_then(Value::as_str)
            .unwrap_or("unknown nvs error");
        Err(NvsError::Nvs(msg.to_string()))
    }
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Error type for the bindings.
#[derive(Debug)]
pub enum NvsError {
    /// NvS itself reported an error (parse error, runtime error, ...).
    Nvs(String),
    /// The bridge misbehaved (bad envelope, null pointer, ...).
    Bridge(String),
    /// An input string contained an interior NUL byte.
    Nul(NulError),
}

impl std::fmt::Display for NvsError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            NvsError::Nvs(m) => write!(f, "nvs error: {m}"),
            NvsError::Bridge(m) => write!(f, "bridge error: {m}"),
            NvsError::Nul(e) => write!(f, "nul byte in input: {e}"),
        }
    }
}

impl std::error::Error for NvsError {}

impl From<NulError> for NvsError {
    fn from(e: NulError) -> Self {
        NvsError::Nul(e)
    }
}

/// Evaluate NvS source in the persistent interpreter. Definitions made by
/// one call are visible to later calls. Returns the decoded `result` value.
pub fn eval(src: &str) -> Result<Value, NvsError> {
    let c_src = CString::new(src)?;
    let _guard = NVS_LOCK.lock().unwrap();
    // SAFETY: c_src is a valid NUL-terminated string; the bridge copies it.
    let ptr = unsafe { nvs_eval(c_src.as_ptr()) };
    let raw = unsafe { take_envelope(ptr)? };
    decode_envelope(&raw)
}

/// Call a named NvS function (or builtin) with JSON arguments, e.g.
/// `nvs::call("add", &json!([20, 22]))`. Arguments are positional only.
pub fn call(name: &str, args: &Value) -> Result<Value, NvsError> {
    let c_name = CString::new(name)?;
    let args_json = serde_json::to_string(args)
        .map_err(|e| NvsError::Bridge(format!("cannot encode args: {e}")))?;
    let c_args = CString::new(args_json)?;
    let _guard = NVS_LOCK.lock().unwrap();
    // SAFETY: both pointers are valid borrowed NUL-terminated strings.
    let ptr = unsafe { nvs_call(c_name.as_ptr(), c_args.as_ptr()) };
    let raw = unsafe { take_envelope(ptr)? };
    decode_envelope(&raw)
}
