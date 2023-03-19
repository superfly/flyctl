package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func fileExists(filenames ...string) checkFn {
	return func(dir string) bool {
		for _, filename := range filenames {
			info, err := os.Stat(filepath.Join(dir, filename))
			if err != nil {
				continue
			}
			if !info.IsDir() {
				return true
			}
		}
		return false
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
