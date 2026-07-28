//go:build windows

package processutil

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// ConfigureBackground prevents console-backed child processes from flashing a
// terminal window when they are launched by the Wails GUI.
func ConfigureBackground(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	command.SysProcAttr.CreationFlags |= createNoWindow
}
