package multilogs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/repo"
)

// baiscally copy'd from sbot sorryyy
type multiCloser struct {
	cs []io.Closer
	l  sync.Mutex
}

func (mc *multiCloser) addCloser(c io.Closer) {
	mc.l.Lock()
	defer mc.l.Unlock()

	mc.cs = append(mc.cs, c)
}

func (mc *multiCloser) Close() error {
	var err error

	mc.l.Lock()
	defer mc.l.Unlock()

	for i, c := range mc.cs {
		err = JoinErrors(err, errors.Wrapf(c.Close(), "multiCloser: close i=%d failed", i))
	}

	return err
}

type newErrorSinkFunc func() chan<- error

func newErrorCollector() (<-chan error, newErrorSinkFunc) {
	var (
		src = make(chan error)
		wg  sync.WaitGroup
	)

	go func() {
		wg.Wait()
		close(src)
		src = nil
	}()

	return src, func() chan<- error {
		if src == nil {
			return nil
		}

		snk := make(chan error)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := <-snk; err != nil {
				src <- err
			}
		}()

		return snk
	}
}

type ServeFunc func(ctx context.Context, log margaret.Log) error

//type NewFunc func(repo.Interface) (margaret.Multilog, ServeFunc, error)

type Manager interface {
	Serve(ctx context.Context) error

	UserFeeds() multilog.MultiLog
	MessageTypes() multilog.MultiLog

	Close() error
}

func NewManager(r repo.Interface, log margaret.Log) (Manager, error) {
	uf, _, serveUF, err := OpenUserFeeds(r)
	if err != nil {
		return nil, errors.Wrap(err, "multilogs manager: failed to open user feeds")
	}

	mt, _, serveMT, err := OpenMessageTypes(r)
	if err != nil {
		return nil, errors.Wrap(err, "multilogs manager: failed to open message types")
	}

	return &manager{
		repo: r,
		log:  log,
		uf:   uf,
		mt:   mt,
		serves: []ServeFunc{
			serveUF,
			serveMT,
		},
		mc: &multiCloser{
			cs: []io.Closer{
				uf,
				mt,
			},
		},
	}, nil
}

type manager struct {
	serves []ServeFunc
	uf     multilog.MultiLog
	mt     multilog.MultiLog
	log    margaret.Log
	repo   repo.Interface
	mc     *multiCloser
}

func (mgr *manager) Serve(ctx context.Context) error {
	errSrc, newSink := newErrorCollector()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, f := range mgr.serves {
		go func(g ServeFunc) {
			newSink() <- g(ctx, mgr.log)
		}(f)
	}

	var me MultiError

	for err := range errSrc {
		me = append(me, err)
		cancel()
	}

	return me
}

func (mgr *manager) MessageTypes() multilog.MultiLog {
	return mgr.mt
}

func (mgr *manager) UserFeeds() multilog.MultiLog {
	return mgr.uf
}

func (mgr *manager) Close() error {
	return mgr.mc.Close()
}

type MultiError []error

func (me MultiError) Error() string {
	if len(me) == 0 {
		return ""
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d errors occurred:\n", len(me))
	for _, err := range me {
		fmt.Fprintf(&b, " - %s", err.Error())
	}

	return b.String()
}

func JoinErrors(err1 error, err2 error) error {
	if err1 == nil {
		return err2
	}

	if err2 == nil {
		return err1
	}

	me1, ok1 := err1.(MultiError)
	me2, ok2 := err2.(MultiError)

	type twobit struct {
		one, two bool
	}

	switch (twobit{ok1, ok2}) {
	case twobit{false, false}:
		return MultiError{err1, err2}
	case twobit{true, false}:
		return append(me1, err2)
	case twobit{false, true}:
		return append(me2, err1)
	case twobit{true, true}:
		return append(me1, me2...)
	}

	panic("unreachable")
}
