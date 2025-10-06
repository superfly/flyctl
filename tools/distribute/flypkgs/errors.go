package flypkgs

import (
	"fmt"
	"net/http"
	"strings"
)

type ErrorResponse struct {
	Code     int
	Message  string   `json:"error"`
	Messages []string `json:"errors"`
}

func (e ErrorResponse) Error() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "API error: %d\n", e.Code)
	for _, msg := range e.Messages {
		fmt.Fprintf(&sb, "  - %s\n", msg)
	}

	return sb.String()
}

func IsNotFoundErr(err error) bool {
	if err == nil {
		return false
	}

	if e, ok := err.(ErrorResponse); ok {
		return e.Code == http.StatusNotFound
	}

	return false
}

func IsConflictError(err error) bool {
	if err == nil {
		return false
	}

	if e, ok := err.(ErrorResponse); ok {
		return e.Code == http.StatusConflict
	}

	return false
}
