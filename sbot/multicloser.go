// SPDX-License-Identifier: MIT

package sbot

import (
	"fmt"
	"io"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
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

	for i, c := range mc.cs {
		if cerr := c.Close(); cerr != nil {
			err = multierror.Append(err, fmt.Errorf("multiCloser: c%d failed: %w", i, cerr))
		}
	}

	me := err.(*multierror.Error)
	if len(me.Errors) == 0 {
		return nil
	}

	return err
}
