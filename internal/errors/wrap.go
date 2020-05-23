package errors

import (
	"github.com/pkg/errors"
)

// Wrap is an alias for "github.com/pkg/errors".Wrap
func Wrap(err error, message string) error {
	return errors.Wrap(err, message)
}

// Wrapf is an alias for "github.com/pkg/errors".Wrapf
func Wrapf(err error, format string, args ...interface{}) error {
	return errors.Wrapf(err, format, args...)
}
