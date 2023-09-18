package flaps

import (
	"encoding/json"
	"net/http"
)

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
	var errResp errorResponse
	unmarshalErr := json.Unmarshal(fe.ResponseBody, &errResp)

	if unmarshalErr != nil {
		return fe.OriginalError.Error()
	}

	switch errResp.StatusCode {
	case unknown:
		return errResp.Error
	case capacityErr:
		if err, ok := errResp.Details.(LaunchCapacityErr); ok {
			return err.Error()
		}
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

func (fe *FlapsError) ErrRequestID() string {
	return fe.FlyRequestId
}

func (fe *FlapsError) ResponseBodyString() string {
	return string(fe.ResponseBody)
}
