// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package keys

import (
	"errors"
	"fmt"
)

// ErrorCode is part of this packages Error type to signal a few specific errors
type ErrorCode uint8

// The known error codes
const (
	ErrorCodeInternal ErrorCode = iota
	ErrorCodeInvalidKeyScheme
	ErrorCodeNoSuchKey
)

func (code ErrorCode) String() string {
	switch code {
	case ErrorCodeInternal:
		return "keys: internal error"
	case ErrorCodeNoSuchKey:
		return "keys: no such key found"
	case ErrorCodeInvalidKeyScheme:
		return "keys: invalid scheme"
	default:
		panic("unhandled error code")
	}
}

// Error is returned by the key store system
type Error struct {
	Code   ErrorCode
	Scheme KeyScheme
	ID     ID

	Cause error
}

func (err Error) Unwrap() error {
	return err.Cause
}

func (err Error) Error() string {
	if err.Code == ErrorCodeInternal {
		return err.Cause.Error()
	}

	return fmt.Sprintf("%s at (%s, %x)", err.Code, err.Scheme, err.ID)
}

// IsNoSuchKey returns true if the error contains a Error from this package with the code ErrorCodeNoSuchKey
func IsNoSuchKey(err error) bool {
	var keysErr Error
	if !errors.As(err, &keysErr) {
		return false
	}
	return keysErr.Code == ErrorCodeNoSuchKey
}
