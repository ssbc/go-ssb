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

	var closeErr error

	closeCh := make(chan struct{})

	return &chanSource{
			ch:          ch,
			closeCh:     closeCh,
			closeErr:    &closeErr,
			nonBlocking: pOpts.nonBlocking,
		}, &chanSink{
			ch:          ch,
			closeErr:    &closeErr,
			closeCh:     closeCh,
			nonBlocking: pOpts.nonBlocking,
		}
}

type chanSource struct {
	ch          <-chan interface{}
	nonBlocking bool
	closeCh     chan struct{}
	closeErr    *error
}

func (src *chanSource) Next(ctx context.Context) (v interface{}, err error) {
	if src.nonBlocking {
		select {
		case v = <-src.ch:
		case <-src.closeCh:
			select {
			case v = <-src.ch:
			default:
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
		case v = <-src.ch:
		case <-src.closeCh:
			select {
			case v = <-src.ch:
			default:
				if *(src.closeErr) != nil {
					err = *(src.closeErr)
				} else {
					err = EOS{}
				}
			}
		case <-ctx.Done():
			// even if both the context is cancelled and the stream is closed,
			// we consistently return the closing error
			select {
			case <-src.closeCh:
				if *(src.closeErr) != nil {
					err = *(src.closeErr)
				} else {
					err = EOS{}
				}
			default:
				err = errors.Wrap(ctx.Err(), "luigi next done")
			}
		}
	}

	return v, err
}

type chanSink struct {
	ch          chan<- interface{}
	nonBlocking bool
	closeLock   sync.Mutex
	closeCh     chan struct{}
	closeErr    *error
	closeOnce   sync.Once
}

func (sink *chanSink) Pour(ctx context.Context, v interface{}) error {
	select {
	case <-sink.closeCh:
		// TODO export error
		return errors.New("pour to closed sink")
	default:
	}

	if sink.nonBlocking {
		select {
		case sink.ch <- v:
			return nil
		case <-sink.closeCh:
			// we may be called with closed context on a closed sink. in that case we want to return the closed sink error.
			select {
			case <-sink.closeCh:
				// TODO export error
				return errors.New("pour to closed sink")
			default:
				return ctx.Err()
			}
		default:
			return errors.New("channel not ready for writing")
		}
	} else {
		select {
		case sink.ch <- v:
			return nil
		case <-sink.closeCh:
			// TODO export error
			return errors.New("pour to closed sink")
		case <-ctx.Done():
			// we may be called with closed context on a closed sink. in that case we want to return the closed sink error.
			select {
			case <-sink.closeCh:
				// TODO export error
				return errors.New("pour to closed sink")
			default:
				return ctx.Err()
			}
		}
	}

}

func (sink *chanSink) Close() error {
	return sink.CloseWithError(EOS{})
}

func (sink *chanSink) CloseWithError(err error) error {
	sink.closeOnce.Do(func() {
		*sink.closeErr = err
		close(sink.closeCh)
	})
	return nil
}
