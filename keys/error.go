package keys

import "fmt"

type ErrorCode uint8

const (
	ErrorCodeInternal ErrorCode = iota
	ErrorCodeNoSuchKey
)

func (code ErrorCode) String() string {
	switch code {
	case ErrorCodeInternal:
		return "internal keys error"
	case ErrorCodeNoSuchKey:
		return "no such key"
	default:
		return ""
	}
}

type Error struct {
	Code ErrorCode
	Type Type
	ID   ID

	Cause error
}

func (err Error) Error() string {
	if err.Code == ErrorCodeInternal {
		return err.Cause.Error()
	}

	return fmt.Sprintf("%s at (%s, %x)", err.Code, err.Type, err.ID)
}

func IsNoSuchKey(err error) bool {
	if err_, ok := err.(Error); !ok {
		return false
	} else {
		return err_.Code == ErrorCodeNoSuchKey
	}
}
