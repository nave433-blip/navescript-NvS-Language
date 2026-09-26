package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubBinary writes a shell script acting as a fake `nvs` binary:
// exits 0 unless the file argument contains "fail".
func stubBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "nvs-stub")
	script := "#!/bin/sh\ncase \"$2\" in\n*fail*) echo \"boom\"; exit 1;;\n*) echo \"stub-ok $2\"; exit 0;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverTests(t *testing.T) {
	dir := t.TempDir()
	files := []string{"test_foo.nvs", "bar_test.ns", "notes.nvs", "test_x.txt", "sub/test_deep.nvs"}
	for _, f := range files {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("print 1"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := DiscoverTests([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 test files, got %v", got)
	}
	for _, g := range got {
		base := filepath.Base(g)
		if !(strings.HasPrefix(base, "test_") || strings.Contains(base, "_test.")) {
			t.Fatalf("non-test file discovered: %s", g)
		}
	}
}

func TestRunTestFilePassFail(t *testing.T) {
	NvsBin = stubBinary(t)
	defer func() { NvsBin = "" }()
	var out bytes.Buffer
	pass := RunTestFile("/tmp/test_ok.nvs", 5*time.Second, false, &out)
	if !pass.Passed {
		t.Fatalf("expected pass, got output %q", pass.Output)
	}
	fail := RunTestFile("/tmp/test_fail.nvs", 5*time.Second, false, &out)
	if fail.Passed {
		t.Fatal("expected fail")
	}
	if !strings.Contains(fail.Output, "boom") {
		t.Fatalf("expected captured output, got %q", fail.Output)
	}
}

func TestRunTestSuiteSummary(t *testing.T) {
	NvsBin = stubBinary(t)
	defer func() { NvsBin = "" }()
	dir := t.TempDir()
	for _, f := range []string{"test_a.nvs", "test_b_fail.nvs"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	failures := RunTestSuite([]string{dir}, 5*time.Second, false, &out)
	if failures != 1 {
		t.Fatalf("want 1 failure, got %d\n%s", failures, out.String())
	}
	if !strings.Contains(out.String(), "1 passed, 1 failed") {
		t.Fatalf("missing summary line:\n%s", out.String())
	}
}

func TestProfileFile(t *testing.T) {
	dir := t.TempDir()
	code := "let total = 0\nfor (i in range(100)) {\n  total = total + i\n}\nprint total\n"
	path := filepath.Join(dir, "hot.nvs")
	if err := os.WriteFile(path, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := ProfileFile(path, 3, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "statement executions") {
		t.Fatalf("missing profile header:\n%s", s)
	}
	// The loop-body line (line 3) must be the hottest with ~100 hits.
	lines := strings.Split(s, "\n")
	found := false
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "3 ") && strings.Contains(ln, "100") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected line 3 with 100 hits at top:\n%s", s)
	}
}
