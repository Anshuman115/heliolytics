package store

import (
	"errors"
	"fmt"
)

// ErrSessionConflict marks an immutable envelope or candidate retry that
// disagrees with stored state. B06 maps it to 409 session_conflict.
var ErrSessionConflict = errors.New("session conflict")

// InvalidRowError marks caller rows the store refuses before any write.
type InvalidRowError struct {
	Table  string
	Reason string
}

func (e *InvalidRowError) Error() string {
	return fmt.Sprintf("%s: %s", e.Table, e.Reason)
}

func invalidRow(table, format string, args ...any) error {
	return &InvalidRowError{Table: table, Reason: fmt.Sprintf(format, args...)}
}
