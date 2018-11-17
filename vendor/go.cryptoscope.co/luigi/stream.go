package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"

	"github.com/pkg/errors"
)

type EOS struct{}

func (_ EOS) Error() string { return "end of stream" }
func IsEOS(err error) bool {
	err = errors.Cause(err)

	_, ok := err.(EOS)
	return ok
}

type Sink interface {
	Pour(context.Context, interface{}) error
	Close() error
}

type ErrorCloser interface {
	CloseWithError(error) error
}

type Source interface {
	Next(context.Context) (interface{}, error)
}

type PushSource interface {
	Push(ctx context.Context, sink Sink) error
}

// Pump moves values from a source into a sink.
//
// Currently this doesn't work atomically, so if a Sink errors in the
// Pour call, the value that was read from the source is lost.
func Pump(ctx context.Context, sink Sink, src Source) error {
	if psrc, ok := src.(PushSource); ok {
		return psrc.Push(ctx, sink)
	}

	for {
		v, err := src.Next(ctx)
		if IsEOS(err) {
			return nil
		} else if err != nil {
			return err
		}

		err = sink.Pour(ctx, v)
		if err != nil {
			return err
		}
	}

	panic("unreachable")
}
