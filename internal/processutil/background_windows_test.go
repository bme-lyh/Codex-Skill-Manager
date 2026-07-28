//go:build windows

package processutil

import (
	"os/exec"
	"testing"
)

func TestConfigureBackgroundHidesConsoleWindow(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	ConfigureBackground(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("expected HideWindow to be enabled")
	}
	if command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("expected CREATE_NO_WINDOW to be enabled")
	}
}
