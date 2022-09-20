package melody

import "errors"

var (
	ErrClosed        = errors.New("")
	ErrSessionClosed = errors.New("session is closed")
)
