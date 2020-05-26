package api

import "errors"

// ErrNotFound - Error to return when something is not found
var ErrNotFound = errors.New("Not Found")

// ErrUnknown - Error to return when an unknown server error occurs
var ErrUnknown = errors.New("An unknown server error occured, please try again")
