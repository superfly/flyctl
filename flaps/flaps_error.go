package flaps

import (
	"encoding/json"
	"errors"
	"net/http"
)

var (
	FlapsErrorNotFound = &FlapsError{ResponseStatusCode: http.StatusNotFound}
)

type StatusCode string

const (
	unknown          StatusCode = "unknown"
	regionOOCapacity StatusCode = "insufficient_capacity"
)

type errorResponse struct {
	Error      string     `json:"error"`
	StatusCode StatusCode `json:"status"`
}

type FlapsError struct {
	OriginalError      error
	ResponseStatusCode int
	ResponseBody       []byte
	FlyRequestId       string
}

func (fe *FlapsError) Error() string {
	if fe.OriginalError == nil {
		return ""
	}
	return fe.OriginalError.Error()
}

func (fe *FlapsError) Suggestion() string {
	statusCode := fe.StatusCode()
	if statusCode == nil {
		return ""
	}

	switch *statusCode {
	case unknown:
		return ""
	case regionOOCapacity:
		// TODO(billy): once we have support for 'backup regions', suggest creating adding those (or eveven better, just do it automatically)
		return "Try choosing a different region for machine creation"
	}

	return ""
}

type ErrorStatusCode interface {
	error
	StatusCode() *StatusCode
}

func GetErrorStatusCode(err error) *StatusCode {
	var ferr ErrorStatusCode
	if errors.As(err, &ferr) {
		return ferr.StatusCode()
	}
	return nil
}

// TODO: we might not actually need an interface type here
type ErrorRequestID interface {
	ErrRequestID() string
}

func GetErrorRequestID(err error) string {
	var ferr ErrorRequestID
	if errors.As(err, &ferr) {
		return ferr.ErrRequestID()
	}
	return ""
}

func (fe *FlapsError) StatusCode() *StatusCode {
	var errResp errorResponse
	unmarshalErr := json.Unmarshal(fe.ResponseBody, &errResp)

	if unmarshalErr != nil {
		return nil
	}

	return &errResp.StatusCode
}

func (fe *FlapsError) ErrRequestID() string {
	return fe.FlyRequestId
}

func (fe *FlapsError) Is(target error) bool {
	if other, ok := target.(*FlapsError); ok {
		return fe.ResponseStatusCode == other.ResponseStatusCode
	}
	return false
}

func (fe *FlapsError) Unwrap() error {
	return fe.OriginalError
}

func (fe *FlapsError) ResponseBodyString() string {
	return string(fe.ResponseBody)
}
