package recipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type Error struct {
	StatusCode int
	Err        string `json:"error"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.StatusCode, e.Err)
}

func ErrorStatus(err error) int {
	var e *Error

	if errors.As(err, &e) {
		return e.StatusCode
	}
	return http.StatusInternalServerError
}

func newError(status int, res *http.Response) error {
	e := new(Error)

	e.StatusCode = status

	switch res.Header.Get("Content-Type") {
	case "application/json":

		if err := json.NewDecoder(res.Body).Decode(e); err != nil {
			return err
		}
	default:
		b, err := io.ReadAll(res.Body)
		if err != nil {
			return err
		}
		e.Err = string(b)
	}

	return e
}
