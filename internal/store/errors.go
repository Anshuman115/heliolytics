package store

import "fmt"

func errRequired(column string) error {
	return fmt.Errorf("%s required", column)
}
