package helpers

import (
	"fmt"
	"strconv"
)

const (
	TB = 1099511627776
	GB = 1073741824
	MB = 1048576
	KB = 1024
)

func BytesToHumanReadable(bytes, decimals int) string {
	var unit string
	var val int
	var remainder int

	if bytes > TB {
		unit = "TB"
		val = bytes / TB
		remainder = bytes - (val * TB)
	} else if bytes > GB {
		unit = "GB"
		val = bytes / GB
		remainder = bytes - (val * GB)
	} else if bytes > MB {
		unit = "MB"
		val = bytes / MB
		remainder = bytes - (val * MB)
	} else if bytes > KB {
		unit = "KB"
		val = val / KB
		remainder = bytes - (val * KB)
	} else {
		unit = "B"
		val = bytes
	}

	if decimals == 0 {
		return strconv.Itoa(val) + " " + unit
	}

	// This is to calculate missing leading zeroes
	width := 0
	if remainder > GB {
		width = 12
	} else if remainder > MB {
		width = 9
	} else if remainder > KB {
		width = 6
	} else {
		width = 3
	}

	// Insert missing leading zeroes
	remainderString := strconv.Itoa(remainder)
	for iter := len(remainderString); iter < width; iter++ {
		remainderString = "0" + remainderString
	}
	if decimals > len(remainderString) {
		decimals = len(remainderString)
	}

	return fmt.Sprintf("%d.%s %s", val, remainderString[:decimals], unit)
}
