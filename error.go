package sio

import "fmt"

var ErrAckTimeout = fmt.Errorf("ack timeout")

// This is a wrapper for the errors internal to socket.io.
//
// If you see this error, this means that the problem is
// neither a network error, nor an error caused by you, but
// the source of the error is socket.io. Open an issue on GitHub.
type InternalError struct {
	err error
}

func (e InternalError) Error() string {
	return "sio: internal error: " + e.err.Error()
}

func (e InternalError) Unwrap() error {
	return e.err
}

func wrapInternalError(err error) *InternalError {
	return &InternalError{err: err}
}
