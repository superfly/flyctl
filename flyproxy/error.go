package flyproxy

type Error struct {
	OriginalError      error
	ResponseStatusCode int
	ResponseBody       []byte
	FlyRequestId       string
}

func (fe *Error) Error() string {
	if fe.OriginalError == nil {
		return ""
	}
	return fe.OriginalError.Error()
}

func (fe *Error) Unwrap() error {
	return fe.OriginalError
}

func (fe *Error) ResponseBodyString() string {
	return string(fe.ResponseBody)
}
