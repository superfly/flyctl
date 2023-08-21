package flypkgs

import (
	"fmt"
	"net/http"
)

type ErrorResponse struct {
	Code    int
	Message string `json:"error"`
}

func (e ErrorResponse) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
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
