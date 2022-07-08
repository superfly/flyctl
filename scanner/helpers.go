package scanner

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
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

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		re := regexp.MustCompile(pattern)
		if re.MatchString(scanner.Text()) {
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

func commentLineInFile(path string, search string) {
	input, err := ioutil.ReadFile(path)

	if err != nil {
		log.Fatalln(err)
	}

	lines := strings.Split(string(input), "\n")

	for i, line := range lines {
		if strings.Contains(line, search) {
			lines[i] = fmt.Sprintf("#%s", line)
		}
	}
	output := strings.Join(lines, "\n")
	err = ioutil.WriteFile(path, []byte(output), 0644)
	if err != nil {
		log.Fatalln(err)
	}
}
