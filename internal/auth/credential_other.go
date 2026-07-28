//go:build !windows

package auth

import "errors"

func ReadGitHubToken() (string, error) { return "", nil }
func SaveGitHubToken(token, username string) error {
	return errors.New("credential storage is only supported on Windows")
}
