package helpers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func IsTerminal() bool {
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) != 0 {
		return true
	}

	return false
}

func ReadStdin(maxLength int) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var output []rune

	bytesRead := 0

	for {
		input, size, err := reader.ReadRune()
		if err != nil && err == io.EOF {
			break
		} else if err != nil {
			return "", err
		}
		bytesRead += size
		if bytesRead > maxLength {
			return "", fmt.Errorf("Input exceeded max length of %d bytes", maxLength)
		}
		output = append(output, input)
	}

	return strings.TrimSpace(string(output)), nil
}

// HasPipedStdin returns if stdin has piped input
func HasPipedStdin() bool {
	stat, _ := os.Stdin.Stat()
	return (stat.Mode() & os.ModeCharDevice) == 0
}
