//go:build windows

package eval

import "os/exec"

// shellCommand builds the platform shell invocation for a command line.
// On NT kernels there is no sh; cmd /c is the equivalent. The sh() and
// system() builtins go through this helper so NvS scripts run unmodified
// on Windows.
func shellCommand(line string) *exec.Cmd {
	return exec.Command("cmd", "/c", line)
}

// shellName reports which shell the sh()/system() builtins use.
func shellName() string { return "cmd" }
