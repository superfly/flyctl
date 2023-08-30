package flyerr

import (
	"context"
	"errors"
	"fmt"

	"github.com/logrusorgru/aurora"
)

type GenericErr struct {
	Err      string
	Descript string
	Suggest  string
	DocUrl   string
}

func (e GenericErr) Error() string {
	return e.Err
}

func (e GenericErr) FlyDocURL() string {
	return e.DocUrl
}

func (e GenericErr) Suggestion() string {
	return e.Suggest
}

func (e GenericErr) Description() string {
	return e.Descript
}

type FlyDocUrl interface {
	DocURL() string
}

func GetErrorDocUrl(err error) string {
	var ferr FlyDocUrl
	if errors.As(err, &ferr) {
		return ferr.DocURL()
	}
	return ""
}

// ErrAbort is an error for when the CLI aborts
var ErrAbort = errors.New("abort")

// ErrorDescription is an error with a detailed description that will be printed before the CLI exits
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

// ErrorSuggestion is an error with suggested next steps that will be printed before the CLI exits
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
		fmt.Printf("\n\n%s", description)
	}

	if suggestion != "" {
		if description != "" {
			fmt.Println()
		}
		fmt.Printf("\n%s", suggestion)
	}
	fmt.Println()
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
