package apperr

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound indicates a requested entity was not found.
	ErrNotFound = errors.New("not found")

	// ErrKeyReused indicates an idempotency key was reused with a different payload.
	ErrKeyReused = errors.New("idempotency key reused with different payload")

	// ErrInvalidState indicates an operation cannot be performed in the current resource state.
	ErrInvalidState = errors.New("invalid state")

	// ErrInvalidInput indicates bad request data or validation failure.
	ErrInvalidInput = errors.New("invalid input")
)

// ErrOverReceipt indicates that received quantity would exceed the issued quantity.
// It carries the remaining outstanding quantity.
type ErrOverReceipt struct {
	Outstanding int
}

func (e ErrOverReceipt) Error() string {
	return fmt.Sprintf("over receipt: outstanding quantity is %d", e.Outstanding)
}

// Is allows errors.Is comparison for ErrOverReceipt without matching the exact Outstanding value.
func (e ErrOverReceipt) Is(target error) bool {
	_, ok := target.(ErrOverReceipt)
	return ok
}
