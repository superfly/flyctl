package flyerr

import (
	"context"
	"errors"
	"fmt"

	"github.com/logrusorgru/aurora"
)

// ErrAbort is an error for when the CLI aborts
var ErrAbort = errors.New("abort")

// ErrorDescription is an error with detailed description that will be printed before the CLI exits
type ErrorDescription interface {
	error
	Description() string
}

func GetErrorDescription(err error) string {
	var ferr ErrorDescription
	if errors.As(err, &ferr) {
		return ferr.Description()
	}
	return ""
}

// ErrorSuggestion is an error with a suggested next steps that will be printed before the CLI exits
type ErrorSuggestion interface {
	error
	Suggestion() string
}

func GetErrorSuggestion(err error) string {
	var ferr ErrorSuggestion
	if errors.As(err, &ferr) {
		return ferr.Suggestion()
	}
	return ""
}

func PrintCLIOutput(err error) {
	if err == nil {
		return
	}

	if IsCancelledError(err) {
		return
	}

	fmt.Println()
	fmt.Println(aurora.Red("Error"), err)

	description := GetErrorDescription(err)
	suggestion := GetErrorSuggestion(err)

	if description != "" {
		fmt.Printf("\n%s", description)
	}

	if suggestion != "" {
		if description != "" {
			fmt.Println()
		}
		fmt.Printf("\n%s", suggestion)
	}
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
