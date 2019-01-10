package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"
	"sync"

	"github.com/pkg/errors"
)

type pipeOpts struct {
	bufferSize  int
	nonBlocking bool
}

// PipeOpt configures NewPipes behavior
type PipeOpt func(*pipeOpts) error

// WithBuffer sets the buffer size of the internal channel
func WithBuffer(bufSize int) PipeOpt {
	return PipeOpt(func(opts *pipeOpts) error {
		opts.bufferSize = bufSize
		return nil
	})
}

// NonBlocking changes the behavior to assume a non-blocking backing medium
func NonBlocking() PipeOpt {
	return PipeOpt(func(opts *pipeOpts) error {
		opts.nonBlocking = true
		return nil
	})
}

// NewPipe returns both ends of a tube
func NewPipe(opts ...PipeOpt) (Source, Sink) {
	var pOpts pipeOpts

	for i, opt := range opts {
		err := opt(&pOpts)
		if err != nil {
			// TODO what to do?
			// the current options don't trigger this anyway
			panic(errors.Wrapf(err, "luigi: invalid pipe option %d", i))
		}
	}

	ch := make(chan interface{}, pOpts.bufferSize)

	var closeLock sync.Mutex
	var closeErr error

	return &chanSource{
			ch:          ch,
			closeLock:   &closeLock,
			closeErr:    &closeErr,
			nonBlocking: pOpts.nonBlocking,
		}, &chanSink{
			ch:          ch,
			closeLock:   &closeLock,
			closeErr:    &closeErr,
			nonBlocking: pOpts.nonBlocking,
		}
}

type chanSource struct {
	ch          <-chan interface{}
	nonBlocking bool
	closeLock   *sync.Mutex
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
			err = errors.Wrap(ctx.Err(), "luigi next done")
			/*
				src.closeLock.Lock()
				err = errors.Wrapf(ctx.Err(), "luigi next done (closed: %v)", *(src.closeErr))
				src.closeLock.Unlock()
			*/
		}
	}

	return v, err
}

type chanSink struct {
	ch          chan<- interface{}
	nonBlocking bool
	closeLock   *sync.Mutex
	closeErr    *error
	closeOnce   sync.Once
}

func (sink *chanSink) Pour(ctx context.Context, v interface{}) error {
	var err error

	sink.closeLock.Lock()
	defer sink.closeLock.Unlock()
	if err := *sink.closeErr; err != nil {
		return err
	}

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
			return errors.Wrapf(ctx.Err(), "luigi pour done (closed: %v)", *(sink.closeErr))
		}
	}

	return err
}

func (sink *chanSink) Close() error {
	return sink.CloseWithError(EOS{})
}

func (sink *chanSink) CloseWithError(err error) error {
	sink.closeOnce.Do(func() {
		sink.closeLock.Lock()
		defer sink.closeLock.Unlock()
		*sink.closeErr = err
		close(sink.ch)
	})
	return nil
}
