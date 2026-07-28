//go:build !windows

package processutil

import "os/exec"

// ConfigureBackground is a no-op on platforms without Windows console windows.
func ConfigureBackground(_ *exec.Cmd) {}
