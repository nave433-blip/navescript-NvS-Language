// build.rs — build libnvs.so from the Go sources and link it.
//
// The NvS repo root is two directories above this crate
// (bindings/rust -> repo root). We run:
//
//   go build -buildmode=c-shared -o $OUT_DIR/libnvs.so ./cbridge
//
// and link the test binaries against it with an rpath into $OUT_DIR,
// so `cargo test` works with no manual setup. Go is located via the
// GO_BIN env var, ~/go-dist/go/bin/go, or PATH, in that order.
use std::env;
use std::path::PathBuf;
use std::process::Command;

fn find_go() -> String {
    if let Ok(p) = env::var("GO_BIN") {
        return p;
    }
    let home = env::var("HOME").unwrap_or_default();
    let cand = PathBuf::from(&home).join("go-dist/go/bin/go");
    if cand.exists() {
        return cand.to_string_lossy().into_owned();
    }
    "go".to_string()
}

fn main() {
    let out_dir = PathBuf::from(env::var("OUT_DIR").unwrap());
    let manifest = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let repo_root = manifest.join("..").join("..");

    let lib = out_dir.join("libnvs.so");
    println!("cargo:rerun-if-changed={}", repo_root.join("cbridge").join("cbridge.go").display());
    println!("cargo:rerun-if-changed={}", manifest.join("build.rs").display());

    let go = find_go();
    let status = Command::new(&go)
        .args(["build", "-buildmode=c-shared", "-o"])
        .arg(&lib)
        .arg("./cbridge")
        .current_dir(&repo_root)
        .status()
        .unwrap_or_else(|e| panic!("failed to run Go toolchain ({go}): {e}"));
    assert!(status.success(), "go build -buildmode=c-shared ./cbridge failed");

    println!("cargo:rustc-link-search=native={}", out_dir.display());
    println!("cargo:rustc-link-lib=dylib=nvs");
    // Bake an rpath so test binaries find libnvs.so at runtime.
    println!(
        "cargo:rustc-link-arg=-Wl,-rpath,{}",
        out_dir.display()
    );
}
