package sprites

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// spriteFileName is the per-directory file that records the active sprite. The
// format is shared with the standalone sprite CLI so that a directory set up
// with either tool works with both.
const spriteFileName = ".sprite"

type spriteFile struct {
	Organization string `json:"organization"`
	Sprite       string `json:"sprite"`
}

// spriteFilePath returns the file to read in dir, or "" when there is none.
// .sprite may be a directory, in which case the selection lives in
// .sprite/selected.
func spriteFilePath(dir string) string {
	path := filepath.Join(dir, spriteFileName)

	info, err := os.Stat(path)
	if err != nil {
		return ""
	}

	if !info.IsDir() {
		return path
	}

	selected := filepath.Join(path, "selected")
	if _, err := os.Stat(selected); err != nil {
		return ""
	}

	return selected
}

// readSpriteFile looks for a .sprite file in dir and each of its parents. It
// returns nil when no file is found.
func readSpriteFile(dir string) (*spriteFile, string, error) {
	for {
		if path := spriteFilePath(dir); path != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, "", fmt.Errorf("failed to read %s: %w", path, err)
			}

			var sf spriteFile
			if err := json.Unmarshal(data, &sf); err != nil {
				return nil, "", fmt.Errorf("failed to parse %s: %w", path, err)
			}

			return &sf, path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", nil
		}

		dir = parent
	}
}

// spriteFileWritePath returns where the selection is written in dir.
func spriteFileWritePath(dir string) string {
	path := filepath.Join(dir, spriteFileName)
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return filepath.Join(path, "selected")
	}

	return path
}

func writeSpriteFile(dir, org, sprite string) error {
	data, err := json.MarshalIndent(spriteFile{Organization: org, Sprite: sprite}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(spriteFileWritePath(dir), data, 0o644)
}

func removeSpriteFile(dir string) error {
	return os.Remove(spriteFileWritePath(dir))
}
