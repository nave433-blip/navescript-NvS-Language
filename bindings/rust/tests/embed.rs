//! Integration tests for the Rust bindings — these link against a real
//! libnvs.so built from the Go sources by build.rs and exercise the
//! eval / call / error paths end to end.
use serde_json::json;

#[test]
fn eval_arithmetic() {
    let v = nvs::eval("6 * 7").expect("eval failed");
    assert_eq!(v, json!(42));
}

#[test]
fn eval_string_result() {
    let v = nvs::eval("\"hello\" + \" \" + \"rust\"").expect("eval failed");
    assert_eq!(v, json!("hello rust"));
}

#[test]
fn define_then_call() {
    nvs::eval("fn radd(a, b) { a + b }").expect("def failed");
    let v = nvs::call("radd", &json!([20, 22])).expect("call failed");
    assert_eq!(v, json!(42));
}

#[test]
fn call_builtin() {
    let v = nvs::call("len", &json!([[1, 2, 3]])).expect("call failed");
    assert_eq!(v, json!(3));
}

#[test]
fn eval_error_propagates() {
    let err = nvs::eval("1 + ").expect_err("expected a parse error");
    match err {
        nvs::NvsError::Nvs(msg) => assert!(msg.contains("parse"), "msg: {msg}"),
        other => panic!("wrong error kind: {other:?}"),
    }
}

#[test]
fn call_unknown_function_errors() {
    let err = nvs::call("no_such_fn_xyz", &json!([])).expect_err("expected an error");
    match err {
        nvs::NvsError::Nvs(msg) => assert!(msg.contains("no_such_fn_xyz"), "msg: {msg}"),
        other => panic!("wrong error kind: {other:?}"),
    }
}

#[test]
fn nested_values_round_trip() {
    let v = nvs::eval("{\"name\": \"nvs\", \"tags\": [\"fast\", \"small\"], \"n\": 7}")
        .expect("eval failed");
    assert_eq!(v, json!({"name": "nvs", "tags": ["fast", "small"], "n": 7}));
}
