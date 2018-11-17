package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

type chanSource struct {
	ch          <-chan interface{}
	nonBlocking bool
	closeErr    *error
}

func (src *chanSource) Next(ctx context.Context) (v interface{}, err error) {
	var ok bool

	if src.nonBlocking {
		select {
		case v, ok = <-src.ch:
			if !ok {
				if *(src.closeErr) != nil {
					err = *(src.closeErr)
				} else {
					err = EOS{}
				}
			}
		default:
			err = errors.New("channel not ready for reading")
		}
	} else {
		select {
		case v, ok = <-src.ch:
			if !ok {
				if *(src.closeErr) != nil {
					err = *(src.closeErr)
				} else {
					err = EOS{}
				}
			}
		case <-ctx.Done():
			err = ctx.Err()
		}
	}

	return v, err
}

type chanSink struct {
	ch          chan<- interface{}
	nonBlocking bool
	closeErr    *error
	closeOnce   sync.Once
}

func (sink *chanSink) Pour(ctx context.Context, v interface{}) error {
	var err error

	if sink.nonBlocking {
		select {
		case sink.ch <- v:
			return nil
		default:
			err = errors.New("channel not ready for writing")
		}
	} else {
		select {
		case sink.ch <- v:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return err
}

func (sink *chanSink) Close() error {
	return sink.CloseWithError(EOS{})
}

func (sink *chanSink) CloseWithError(err error) error {
	sink.closeOnce.Do(func() {
		*sink.closeErr = err
		close(sink.ch)
	})
	return nil
}

type pipeOpts struct {
	bufferSize  int
	nonBlocking bool
}

type PipeOpt func(*pipeOpts) error

func WithBuffer(bufSize int) PipeOpt {
	return PipeOpt(func(opts *pipeOpts) error {
		opts.bufferSize = bufSize
		return nil
	})
}

func NonBlocking() PipeOpt {
	return PipeOpt(func(opts *pipeOpts) error {
		opts.nonBlocking = true
		return nil
	})
}

func NewPipe(opts ...PipeOpt) (Source, Sink) {
	var pOpts pipeOpts

	for _, opt := range opts {
		err := opt(&pOpts)
		if err != nil {
			// TODO what to do?
			panic(err)
		}
	}

	ch := make(chan interface{}, pOpts.bufferSize)

	var closeErr error

	return &chanSource{
			ch:          ch,
			closeErr:    &closeErr,
			nonBlocking: pOpts.nonBlocking,
		}, &chanSink{
			ch:          ch,
			closeErr:    &closeErr,
			nonBlocking: pOpts.nonBlocking,
		}
}
