//go:build windows

package space

import (
	"os"
	"os/exec"
)

func configureProcess(cmd *exec.Cmd) {}

func terminateProcess(process *os.Process, force bool) error {
	return process.Kill()
}
