package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestCapture(t *testing.T) {
	tests := []struct {
		name     string
		initial  error
		errFunc  func() error
		msg      string
		expected string
	}{
		{
			name:     "No error from errFunc",
			initial:  nil,
			errFunc:  func() error { return nil },
			msg:      "test message",
			expected: "",
		},
		{
			name:     "Error from errFunc with no initial error",
			initial:  nil,
			errFunc:  func() error { return errors.New("error from func") },
			msg:      "test message",
			expected: "test message: error from func",
		},
		{
			name:     "Error from errFunc with initial error",
			initial:  errors.New("initial error"),
			errFunc:  func() error { return errors.New("error from func") },
			msg:      "test message",
			expected: "initial error\ntest message: error from func",
		},
		{
			name:     "Error from errFunc with initial wrapped error",
			initial:  fmt.Errorf("wrapped error: %w", errors.New("initial error")),
			errFunc:  func() error { return errors.New("error from func") },
			msg:      "test message",
			expected: "wrapped error: initial error\ntest message: error from func",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error = tt.initial
			Capture(&err, tt.errFunc, tt.msg)
			if err != nil && err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, err.Error())
			} else if err == nil && tt.expected != "" {
				t.Errorf("expected %q, got nil", tt.expected)
			}
		})
	}
}
