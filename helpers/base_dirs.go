package helpers

import (
	"os"
	"path/filepath"
)

// At present, any of these three functions will return the single directory
// where config and state files should be stored, either respecting
// `FLY_CONFIG_DIR` or defaulting to the user's home directory at `~/.fly`. A
// later implementation of the XDG base dir spec might cause them to be
// different.

// The config directory is for "user-specific configuration files"
func GetConfigDirectory() (string, error) {
	if value, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		return value, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".fly"), nil
}

// The state directory is for "data that should persist between application
// restarts, but that is not important or portable", such as "logs, history,
// recently used files, view, layout, open files, undo history, ..."
func GetStateDirectory() (string, error) {
	if value, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		return value, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".fly"), nil
}

// The runtime directory is for "user-specific non-essential runtime files
// and other file objects (such as sockets, named pipes, ...)"
func GetRuntimeDirectory() (string, error) {
	if value, isSet := os.LookupEnv("FLY_CONFIG_DIR"); isSet {
		return value, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".fly"), nil
}
