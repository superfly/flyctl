package flaps

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

func (fe *FlapsError) Unwrap() error {
	return fe.OriginalError
}

func (fe *FlapsError) ResponseBodyString() string {
	return string(fe.ResponseBody)
}
