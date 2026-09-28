//go:build darwin || linux

package checkrunner

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Put the test and ordinary descendants in one group so a deadline bounds
// both the Go test command and child binaries retaining its output pipes.
func configureTermination(command *exec.Cmd, terminate func(*exec.Cmd) error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return terminate(command) }
}

func terminateProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
