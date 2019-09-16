package api

import "errors"

var ErrNotFound = errors.New("Not Found")
var ErrUnknown = errors.New("An unknown server error occured, please try again")
