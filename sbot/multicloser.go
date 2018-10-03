package sbot

import (
	"io"
	"sync"

	"github.com/hashicorp/go-multierror"
)

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

	for _, c := range mc.cs {
		multierror.Append(err, c.Close())
	}

	return err
}
