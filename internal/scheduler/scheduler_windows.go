//go:build windows

package scheduler

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/bme-lyh/Codex-Skill-Manager/internal/processutil"
)

const taskName = "CodexSkillManager-UpdateCheck"

func Configure(executable, configPath, frequency, at string, enabled bool) error {
	if strings.TrimSpace(executable) == "" {
		return fmt.Errorf("executable path is required")
	}
	schedule := "WEEKLY"
	switch strings.ToLower(frequency) {
	case "daily":
		schedule = "DAILY"
	case "weekly":
		schedule = "WEEKLY"
	default:
		return fmt.Errorf("unsupported schedule: %s", frequency)
	}
	command := fmt.Sprintf(`"%s" --config "%s" check --scheduled`, executable, configPath)
	args := []string{"/Create", "/TN", taskName, "/TR", command, "/SC", schedule, "/ST", at, "/F"}
	create := exec.Command("schtasks.exe", args...)
	processutil.ConfigureBackground(create)
	if output, err := create.CombinedOutput(); err != nil {
		return fmt.Errorf("create scheduled task: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !enabled {
		disable := exec.Command("schtasks.exe", "/Change", "/TN", taskName, "/Disable")
		processutil.ConfigureBackground(disable)
		if output, err := disable.CombinedOutput(); err != nil {
			return fmt.Errorf("disable scheduled task: %w: %s", err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
