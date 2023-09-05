package main

import (
	"os/exec"
	"strings"
)

type GitError struct {
	message string
}

func (e GitError) Error() string {
	return "git error: " + e.message
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", GitError{message: string(out)}
	}
	return strings.TrimSpace(string(out)), nil
}
