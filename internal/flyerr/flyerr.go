package flyerr

import (
	"context"
	"errors"
)

// ErrAbort is an error for when the CLI aborts
var ErrAbort = errors.New("abort")
func PrintCLIOutput(err error) {
	if err == nil {
		return
	}

	if IsCancelledError(err) {
		return
	}

	fmt.Println()
	fmt.Println(aurora.Red("Error"), err)
}

func IsCancelledError(err error) bool {
	if errors.Is(err, ErrAbort) {
		return true
	}

	if errors.Is(err, context.Canceled) {
		return true
	}

	// if err == cmd.ErrAbort {
	// 	return true
	// }

	// if err == context.Canceled {
	// 	return true
	// }

	// if merr, ok := err.(*multierror.Error); ok {
	// 	if len(merr.Errors) == 1 && merr.Errors[0] == context.Canceled {
	// 		return true
	// 	}
	// }

	return false
}
