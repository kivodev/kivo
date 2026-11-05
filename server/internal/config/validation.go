package config

import (
	"errors"

	"github.com/mmcdole/kivo/internal/fault"
)

// ErrInvalid is what every *ValidationError wraps.
var ErrInvalid = fault.New(fault.Invalid, "invalid settings")

// ValidationError lists the keys that failed their checks. Each key has one
// error, which names the key.
type ValidationError struct {
	Errs []error
}

func (e *ValidationError) Error() string { return errors.Join(e.Errs...).Error() }

func (e *ValidationError) Unwrap() []error { return append([]error{ErrInvalid}, e.Errs...) }

// invalid returns nil when errs is empty, and otherwise a *ValidationError
// holding errs.
func invalid(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return &ValidationError{Errs: errs}
}
