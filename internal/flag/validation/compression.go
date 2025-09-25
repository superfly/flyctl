package validation

import (
	"fmt"

	"github.com/superfly/flyctl/internal/flyerr"
)

// ValidateCompressionFlag checks if the --compression flag has a valid value.
// This can be "gzip" (soon to be legacy) or "zstd" (what we'd like to be the default)
func ValidateCompressionFlag(compression string) error {
	if compression == "" || compression == "gzip" || compression == "zstd" {
		return nil // Valid
	}

	return flyerr.GenericErr{
		Err:     fmt.Sprintf("Invalid value '%s' for compression. Valid options are 'gzip', 'zstd', or leave unset.", compression),
		Suggest: "Please use 'gzip', 'zstd', or omit the flag.",
	}
}

// ValidateCompressionLevelFlag checks if the --compression-level flag has a value between 0 and 9.
// This is what is currently supported by Builder (they map these to proper zstd compression levels)
func ValidateCompressionLevelFlag(level int) error {
	if level < 0 || level > 9 {
		return flyerr.GenericErr{
			Err:     fmt.Sprintf("Invalid value '%d' for compression level. Must be an integer between 0 and 9.", level),
			Suggest: "Please use an integer between 0 and 9, or omit the flag.",
		}
	}

	return nil
}
