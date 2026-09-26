// Package polyglot runs short code snippets in host language toolchains.
package polyglot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Result is stdout/stderr text from a plugin run.
type Result struct {
	Output string
	Err    error
}

func runCmd(name string, args ...string) Result {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return Result{Output: string(out), Err: err}
}

func withTempDir(prefix string, fn func(dir string) Result) Result {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return Result{Err: err}
	}
	defer os.RemoveAll(dir)
	return fn(dir)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// Python runs python3 -c, falling back to python (stock Windows installs
// provide python.exe, not python3.exe).
func Python(code string) Result {
	bin := "python3"
	if _, err := exec.LookPath(bin); err != nil {
		bin = "python"
	}
	return runCmd(bin, "-c", code)
}

// JS runs node -e
func JS(code string) Result {
	return runCmd("node", "-e", code)
}

// Ruby runs ruby -e
func Ruby(code string) Result {
	return runCmd("ruby", "-e", code)
}

// Rust compiles and runs a rustc binary from a temp main.rs
func Rust(code string) Result {
	return withTempDir("nvs-rust-", func(dir string) Result {
		src := filepath.Join(dir, "main.rs")
		bin := filepath.Join(dir, "main")
		if runtime.GOOS == "windows" {
			bin += ".exe" // rustc emits main.exe on NT; without the suffix exec fails
		}
		body := code
		if !strings.Contains(code, "fn main") {
			body = "fn main() {\n" + code + "\n}\n"
		}
		if err := writeFile(src, body); err != nil {
			return Result{Err: err}
		}
		if r := runCmd("rustc", "-O", "-o", bin, src); r.Err != nil {
			return Result{Output: r.Output, Err: fmt.Errorf("rustc: %w\n%s", r.Err, r.Output)}
		}
		return runCmd(bin)
	})
}

// Go runs go run on a temp main.go
func Go(code string) Result {
	return withTempDir("nvs-go-", func(dir string) Result {
		src := filepath.Join(dir, "main.go")
		body := code
		if !strings.Contains(code, "package ") {
			body = "package main\nimport \"fmt\"\nfunc main() {\n" + code + "\n}\n"
			if strings.Contains(code, "fmt.") {
				// already importing fmt in wrapper
			}
		}
		if err := writeFile(src, body); err != nil {
			return Result{Err: err}
		}
		cmd := exec.Command("go", "run", src)
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
		out, err := cmd.CombinedOutput()
		return Result{Output: string(out), Err: err}
	})
}

// C compiles with gcc and runs
func C(code string) Result {
	return withTempDir("nvs-c-", func(dir string) Result {
		src := filepath.Join(dir, "main.c")
		bin := filepath.Join(dir, "main")
		body := code
		if !strings.Contains(code, "main(") {
			body = "#include <stdio.h>\nint main(void) {\n" + code + "\nreturn 0;\n}\n"
		}
		if err := writeFile(src, body); err != nil {
			return Result{Err: err}
		}
		if r := runCmd("gcc", "-O2", "-o", bin, src); r.Err != nil {
			return Result{Output: r.Output, Err: fmt.Errorf("gcc: %w\n%s", r.Err, r.Output)}
		}
		return runCmd(bin)
	})
}

// CPP compiles with g++ and runs
func CPP(code string) Result {
	return withTempDir("nvs-cpp-", func(dir string) Result {
		src := filepath.Join(dir, "main.cpp")
		bin := filepath.Join(dir, "main")
		body := code
		if !strings.Contains(code, "main(") {
			body = "#include <iostream>\nint main() {\n" + code + "\nreturn 0;\n}\n"
		}
		if err := writeFile(src, body); err != nil {
			return Result{Err: err}
		}
		if r := runCmd("g++", "-O2", "-o", bin, src); r.Err != nil {
			return Result{Output: r.Output, Err: fmt.Errorf("g++: %w\n%s", r.Err, r.Output)}
		}
		return runCmd(bin)
	})
}

// Java compiles and runs a public class Main
func Java(code string) Result {
	return withTempDir("nvs-java-", func(dir string) Result {
		src := filepath.Join(dir, "Main.java")
		body := code
		if !strings.Contains(code, "class ") {
			body = "public class Main {\n  public static void main(String[] args) {\n" + code + "\n  }\n}\n"
		}
		if err := writeFile(src, body); err != nil {
			return Result{Err: err}
		}
		if r := runCmd("javac", src); r.Err != nil {
			return Result{Output: r.Output, Err: fmt.Errorf("javac: %w\n%s", r.Err, r.Output)}
		}
		cmd := exec.Command("java", "-cp", dir, "Main")
		out, err := cmd.CombinedOutput()
		return Result{Output: string(out), Err: err}
	})
}

// CSS performs a lightweight structural check and returns a summary.
// Not a full browser engine — validates braces/parens balance and reports size.
func CSS(code string) Result {
	type stack []rune
	var st stack
	line, col := 1, 0
	for _, ch := range code {
		col++
		if ch == '\n' {
			line++
			col = 0
			continue
		}
		switch ch {
		case '{', '(', '[':
			st = append(st, ch)
		case '}':
			if len(st) == 0 || st[len(st)-1] != '{' {
				return Result{Output: fmt.Sprintf("css: unmatched '}' at line %d col %d\n", line, col), Err: fmt.Errorf("css syntax")}
			}
			st = st[:len(st)-1]
		case ')':
			if len(st) == 0 || st[len(st)-1] != '(' {
				return Result{Output: fmt.Sprintf("css: unmatched ')' at line %d col %d\n", line, col), Err: fmt.Errorf("css syntax")}
			}
			st = st[:len(st)-1]
		case ']':
			if len(st) == 0 || st[len(st)-1] != '[' {
				return Result{Output: fmt.Sprintf("css: unmatched ']' at line %d col %d\n", line, col), Err: fmt.Errorf("css syntax")}
			}
			st = st[:len(st)-1]
		}
	}
	if len(st) != 0 {
		return Result{Output: fmt.Sprintf("css: unclosed %c\n", st[len(st)-1]), Err: fmt.Errorf("css syntax")}
	}
	// count rough rules
	rules := strings.Count(code, "{")
	return Result{Output: fmt.Sprintf("css: ok (%d bytes, ~%d rules)\n", len(code), rules)}
}

// Available reports which toolchains are present.
func Available() string {
	tools := []struct {
		name string
		bin  string
	}{
		{"python", "python3"},
		{"js", "node"},
		{"ruby", "ruby"},
		{"rust", "rustc"},
		{"go", "go"},
		{"c", "gcc"},
		{"cpp", "g++"},
		{"java", "javac"},
		{"css", "builtin"},
	}
	var ok []string
	for _, t := range tools {
		if t.bin == "builtin" {
			ok = append(ok, t.name)
			continue
		}
		if _, err := exec.LookPath(t.bin); err == nil {
			ok = append(ok, t.name)
		}
	}
	return strings.Join(ok, ", ")
}

// Ensure temp cleanup is reasonably fast on failure paths.
var _ = time.Now
