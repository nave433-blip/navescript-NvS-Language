package eval

import (
	"runtime"
	"strings"
	"testing"
)

// TestShellCommandRuns verifies the platform shell helper actually executes
// a command line. On Windows CI this exercises cmd /c; on Unix, sh -c.
// (No Windows hardware in this VM: the windows branch is compile-verified
// via GOOS=windows go build.)
func TestShellCommandRuns(t *testing.T) {
	out, err := shellCommand("echo nvs-shell-ok").Output()
	if err != nil {
		t.Fatalf("shellCommand failed: %v", err)
	}
	if !strings.Contains(string(out), "nvs-shell-ok") {
		t.Fatalf("unexpected shell output %q", out)
	}
}

func TestShellNameMatchesRuntime(t *testing.T) {
	want := "sh"
	if runtime.GOOS == "windows" {
		want = "cmd"
	}
	if shellName() != want {
		t.Fatalf("shellName() = %q, want %q for GOOS=%s", shellName(), want, runtime.GOOS)
	}
}
