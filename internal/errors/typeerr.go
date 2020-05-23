package errors

import "fmt"

type TypeError struct {
	Expected, Actual interface{}
}

func (err TypeError) Error() string {
	return fmt.Sprintf("expected type %T, got %T", err.Expected, err.Actual)
}
