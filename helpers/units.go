package helpers

import (
	"fmt"
	"strconv"
)

func ParseSize(s string, parser func(string) (int64, error), quotient int) (int, error) {
	size, err := strconv.Atoi(s)
	if err != nil {
		sizeBytes, err := parser(s)
		if err != nil {
			return 0, fmt.Errorf("invalid size: %w", err)
		}

		size = int(sizeBytes) / quotient
	}

	return size, nil
}
