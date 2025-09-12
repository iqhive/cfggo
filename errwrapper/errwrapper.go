package errwrapper

import (
	"fmt"
)

type ErrorWrapper func(err error, errorcode int, msg string, args ...interface{}) error

func NewDefaultErrorWrapper() ErrorWrapper {
	return defaultErrorWrapper
}

func defaultErrorWrapper(err error, errorcode int, msg string, args ...interface{}) error {
	if msg == "" {
		return err
	}
	return fmt.Errorf(msg, args...)
}
