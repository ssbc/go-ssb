// +build ignore

package muxrpc

import (
	"errors"
	"fmt"
)

func before(msg string) error {
	return fmt.Errorf(msg)
}

func after(msg string) error {
	return errors.New(msg)
}
