package nave

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runDoc(t *testing.T, doc string) (string, *Runner) {
	t.Helper()
	r := NewRunner(nil)
	d, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := r.Run(d); err != nil {
		t.Fatalf("run: %v", err)
	}
	return "", r
}

func TestNaveLogSetAnswer(t *testing.T) {
	var buf bytes.Buffer
	r := NewRunner(&buf)
	d, _ := Parse([]byte(`{"module":"m","steps":[
		{"op":"log","message":"hi"},
		{"op":"set","var":"a","value":5},
		{"op":"set","var":"b","value":37},
		{"op":"native_op","operator":"add","args":["a","b"],"return_var":"result"},
		{"op":"assert_eq","left":"result","right":42},
		{"op":"answer","var":"result"}
	]}`))
	if err := r.Run(d); err != nil {
		t.Fatalf("run: %v", err)
	}
	if r.Vars["result"] != 42.0 {
		t.Fatalf("result = %v, want 42", r.Vars["result"])
	}
	if !strings.Contains(buf.String(), "hi") {
		t.Fatalf("log output missing: %q", buf.String())
	}
}

func TestNaveAssertFails(t *testing.T) {
	r := NewRunner(nil)
	d, _ := Parse([]byte(`{"module":"m","steps":[
		{"op":"assert_eq","left":1,"right":2}
	]}`))
	if err := r.Run(d); err == nil {
		t.Fatal("want assert_eq failure, got nil")
	}
}

func TestNaveIfBranch(t *testing.T) {
	_, r := runDoc(t, `{"module":"m","steps":[
		{"op":"set","var":"flag","value":true},
		{"op":"if","condition":"flag","then":[{"op":"set","var":"took","value":"yes"}],"else":[{"op":"set","var":"took","value":"no"}]}
	]}`)
	if r.Vars["took"] != "yes" {
		t.Fatalf("took = %v, want yes", r.Vars["took"])
	}
}

func TestNaveFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "w.nave")
	content := `{"module":"filerw","steps":[
		{"op":"file_write","path":"` + filepath.Join(dir, "out.txt") + `","content":"hello nave"},
		{"op":"file_read","path":"` + filepath.Join(dir, "out.txt") + `","return_var":"back"},
		{"op":"assert_eq","left":"back","right":"hello nave"}
	]}`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := RunFile(p, &buf); err != nil {
		t.Fatalf("RunFile: %v", err)
	}
}

func TestNaveUnknownOpSkipped(t *testing.T) {
	var buf bytes.Buffer
	r := NewRunner(&buf)
	d, _ := Parse([]byte(`{"module":"m","steps":[
		{"op":"nasm_exec","code":"whatever"},
		{"op":"bogus_op"}
	]}`))
	if err := r.Run(d); err != nil {
		t.Fatalf("unknown ops should be skipped, got: %v", err)
	}
	if !strings.Contains(buf.String(), "skip") {
		t.Fatalf("want skip log, got %q", buf.String())
	}
}

func TestNaveHelloExample(t *testing.T) {
	var buf bytes.Buffer
	if err := RunFile("../../examples/nave/hello.nave", &buf); err != nil {
		t.Fatalf("RunFile: %v", err)
	}
	if !strings.Contains(buf.String(), "answer result = 42") {
		t.Fatalf("hello.nave output wrong: %q", buf.String())
	}
}
