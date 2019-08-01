package api

import "net/http"

type ApiError struct {
	WrappedError error
	Message      string
	Status       int
}

func (e *ApiError) Error() string { return e.Message }

func ErrorFromResp(resp *http.Response) *ApiError {
	return &ApiError{
		Message: resp.Status,
		Status:  resp.StatusCode,
	}
}

func IsNotAuthenticatedError(err error) bool {
	if apiErr, ok := err.(*ApiError); ok {
		return apiErr.Status == 401
	}
	return false
}

func IsNotFoundError(err error) bool {
	if apiErr, ok := err.(*ApiError); ok {
		return apiErr.Status == 404
	}
	return false
}

func IsServerError(err error) bool {
	if apiErr, ok := err.(*ApiError); ok {
		return apiErr.Status >= 500
	}
	return false
}

func IsClientError(err error) bool {
	if apiErr, ok := err.(*ApiError); ok {
		return apiErr.Status >= 400 && apiErr.Status < 500
	}
	return false
}
