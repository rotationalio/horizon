package auth

import "errors"

var (
	ErrNil          = errors.New("credential is nil")
	ErrWrongType    = errors.New("credential has wrong auth type")
	ErrMissingValue = errors.New("one or more credential values are missing")
)
