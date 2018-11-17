package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"
	"sync"

	"github.com/hashicorp/go-multierror"
)

type Broadcast interface {
	Register(Sink) func()
}

func NewBroadcast() (Sink, Broadcast) {
	bcst := broadcast{sinks: make(map[*Sink]struct{})}

	return (*broadcastSink)(&bcst), &bcst
}

type broadcast struct {
	sync.Mutex
	sinks map[*Sink]struct{}
}

func (bcst *broadcast) Register(sink Sink) func() {
	bcst.Lock()
	defer bcst.Unlock()
	bcst.sinks[&sink] = struct{}{}

	return func() {
		bcst.Lock()
		defer bcst.Unlock()
		delete(bcst.sinks, &sink)
		sink.Close()
	}
}

type broadcastSink broadcast

func (bcst *broadcastSink) Pour(ctx context.Context, v interface{}) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bcst.Lock()
	defer bcst.Unlock()

	sinks := make([]Sink, 0, len(bcst.sinks))

	for sink := range bcst.sinks {
		sinks = append(sinks, *sink)
	}

	// release lock while broadcasting
	// they might want to take it, e.g. to call cancel()
	bcst.Unlock()
	defer bcst.Lock()

	var (
		wg   sync.WaitGroup
		merr *multierror.Error
	)

	wg.Add(len(sinks))
	for _, sink_ := range sinks {
		go func(sink Sink) {
			defer wg.Done()

			err := sink.Pour(ctx, v)
			if err != nil {
				merr = multierror.Append(merr, err)
				return
			}
		}(sink_)
	}

	wg.Wait()

	return merr.ErrorOrNil()
}

func (bcst *broadcastSink) Close() error { return nil }
