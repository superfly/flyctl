package app

import (
	"os"
	"path"
	"path/filepath"
)

func ResolveConfigFileFromPath(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	// Is this a bare directory path? Stat the path
	pd, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return "", err
	}

	// Ok, something exists. Is it a file - yes? return the path
	if pd.IsDir() {
		return path.Join(p, DefaultConfigFileName), nil
	}

	return p, nil
}

func ConfigFileExistsAtPath(p string) (bool, error) {
	p, err := ResolveConfigFileFromPath(p)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	return !os.IsNotExist(err), nil
}
