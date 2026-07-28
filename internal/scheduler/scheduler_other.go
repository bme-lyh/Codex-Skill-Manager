//go:build !windows

package scheduler

import "errors"

func Configure(executable, configPath, frequency, at string, enabled bool) error {
	return errors.New("scheduled checks are supported on Windows only")
}
