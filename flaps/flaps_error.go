package flaps

import "net/http"

var (
	FlapsErrorNotFound = &FlapsError{ResponseStatusCode: http.StatusNotFound}
)

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
