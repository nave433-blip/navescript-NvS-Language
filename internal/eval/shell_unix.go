//go:build !windows

package eval

import "os/exec"

// shellCommand builds the platform shell invocation for a command line.
// On Unix kernels this is sh -c; the Windows counterpart lives in
// shell_windows.go (cmd /c). The sh() and system() builtins go through
// this helper so NvS scripts run unmodified on NT kernels.
func shellCommand(line string) *exec.Cmd {
	return exec.Command("sh", "-c", line)
}

// shellName reports which shell the sh()/system() builtins use.
func shellName() string { return "sh" }
