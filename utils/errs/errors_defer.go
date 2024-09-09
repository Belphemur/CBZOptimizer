package errs

import (
	"errors"
	"fmt"
)

// Capture runs errFunc and assigns the error, if any, to *errPtr. Preserves the
// original error by wrapping with errors.Join if the errFunc err is non-nil.
func Capture(errPtr *error, errFunc func() error, msg string) {
	err := errFunc()
	if err == nil {
		return
	}
	*errPtr = errors.Join(*errPtr, fmt.Errorf("%s: %w", msg, err))
}
