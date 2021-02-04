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

var _ io.Closer = (*multiCloser)(nil)

func (mc *multiCloser) Close() error {
	mc.l.Lock()
	defer mc.l.Unlock()

	var (
		hasErrs bool
		err     error
	)

	for i, c := range mc.cs {
		if cerr := c.Close(); cerr != nil {
			err = multierror.Append(err, fmt.Errorf("multiCloser: c%d failed: %w", i, cerr))
			hasErrs = true
		}
	}

	if !hasErrs {
		return nil
	}

	return err
}
