// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package asynctesting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	librarian "go.cryptoscope.co/margaret/indexes"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func CheckTypes(t *testing.T, tipe string, tExpected []string, rootLog margaret.Log, mt multilog.MultiLog) {
	r := require.New(t)

	typeLog, err := mt.Get(librarian.Addr(tipe))
	r.NoError(err, "error getting contacts log")
	typeLogQuery, err := typeLog.Query()
	r.NoError(err, "error querying contacts log")

	alice1Sink, wait := MakeCompareSink(tExpected, rootLog)
	err = luigi.Pump(context.TODO(), alice1Sink, typeLogQuery)
	r.NoError(err, "error pumping")
	err = alice1Sink.Close()
	r.NoError(err, "error closing sink")
	<-wait
}

func MakeCompareSink(texpected []string, rootLog margaret.Log) (luigi.FuncSink, chan struct{}) {
	var i int
	closeC := make(chan struct{})
	snk := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		defer func() { i++ }()
		var expectEOS = i > len(texpected)-1
		if err != nil {
			if luigi.IsEOS(err) {
				if expectEOS {
					close(closeC)
					return nil
				}
				return fmt.Errorf("expected more values after err %v  i%d, len%d", err, i, len(texpected))
			}
			return fmt.Errorf("unexpected error %q", err)
		}

		v, err = rootLog.Get(v.(int64))
		if err != nil {
			return fmt.Errorf("error getting message from root log %q", v)
		}
		if expectEOS {
			return fmt.Errorf("expected EOS but got value(%d) %v", i, v)
		}

		m := make(map[string]interface{})

		abs := v.(refs.Message)
		err = json.Unmarshal(abs.ContentBytes(), &m)
		if err != nil {
			return fmt.Errorf("error decoding stored message %q", v)
		}
		if got := m["test"]; got != texpected[i] {
			return fmt.Errorf("unexpected value %+v instead of %+v at i=%v", got, texpected[i], i)
		}

		return nil
	})

	return snk, closeC
}

func ServeLog(ctx context.Context, name string, l margaret.Log, snk librarian.SinkIndex, live bool) <-chan error {
	errc := make(chan error)
	go func() {
		defer close(errc)

		src, err := l.Query(margaret.SeqWrap(true), snk.QuerySpec(), margaret.Live(live))
		if err != nil {
			errc <- fmt.Errorf("%s failed to construct query: %w", name, err)
			return
		}

		err = luigi.Pump(ctx, snk, src)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ssb.ErrShuttingDown) {
			errc <- fmt.Errorf("%s serve exited: %w", name, err)
			return
		}
	}()
	return errc
}

func MergedErrors(cs ...<-chan error) <-chan error {
	var wg sync.WaitGroup
	out := make(chan error)

	output := func(c <-chan error) {
		for a := range c {
			out <- a
		}
		wg.Done()
	}

	wg.Add(len(cs))
	for _, c := range cs {
		go output(c)
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
