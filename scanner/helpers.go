package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

func absFileExists(filenames ...string) bool {
	for _, filename := range filenames {
		info, err := os.Stat(filename)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			return true
		}
	}
	return false
}

func fileExists(filenames ...string) checkFn {
	return func(dir string) bool {
		return absFileExists(lo.Map(filenames, func(filename string, _ int) string {
			return filepath.Join(dir, filename)
		})...)
	}
}

func fileContains(path string, pattern string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}

	defer file.Close() //skipcq: GO-S2307

	scanner := bufio.NewScanner(file)

	// Unicode is a complex subject, but if we assume that the pattern is only
	// looking for strings expressible in ASCII, a lot of simplifications can
	// be made.  Most encodings express strings containing only valid ASCII
	// characters the same.  The only encodings that matter that don't are
	// UTF-16, which will add null bytes, either before or after each
	// character.  Since UTF-16 is rare except on Windows, we do a scan to see
	// if we need to allocate a new string.
	re := regexp.MustCompile(pattern)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, "\u0000") {
			text = strings.ReplaceAll(text, "\u0000", "")
		}

		if re.MatchString(text) {
			return true
		}
	}

	return false
}

func dirContains(glob string, patterns ...string) checkFn {
	return func(dir string) bool {
		for _, pattern := range patterns {
			filenames, _ := filepath.Glob(filepath.Join(dir, glob))
			for _, filename := range filenames {
				if fileContains(filename, pattern) {
					return true
				}
			}
		}
		return false
	}
}

type checkFn func(dir string) bool

func checksPass(sourceDir string, checks ...checkFn) bool {
	for _, check := range checks {
		if check(sourceDir) {
			return true
		}
	}
	return false
}

func readTomlFile(file string) (map[string]interface{}, error) {
	doc, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.Wrap(err, "Error reading  "+file)
	}
	tomlData := make(map[string]interface{})
	readErr := toml.Unmarshal(doc, &tomlData)
	if readErr != nil {
		return nil, errors.Wrap(readErr, "Error parsing "+file)
	}
	return tomlData, nil
}
