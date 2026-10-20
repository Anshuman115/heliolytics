package store

import (
	"errors"
	"fmt"
)

var ErrSessionConflict = errors.New("session conflict")

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
