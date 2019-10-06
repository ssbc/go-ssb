// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"sync"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/plugins2"
	"go.cryptoscope.co/ssb/repo"
)

func MountPlugin(plug ssb.Plugin, mode plugins2.AuthMode) Option {
	return func(s *Sbot) error {
		if wrl, ok := plug.(plugins2.NeedsRootLog); ok {
			wrl.WantRootLog(s.RootLog)
		}

		if wrl, ok := plug.(plugins2.NeedsMultiLog); ok {
			err := wrl.WantMultiLog(s)
			if err != nil {
				return errors.Wrap(err, "sbot/mount plug: failed to fulfill multilog requirement")
			}
		}

		if slm, ok := plug.(repo.SimpleIndexMaker); ok {
			err := MountSimpleIndex(plug.Name(), slm.MakeSimpleIndex)(s)
			if err != nil {
				return errors.Wrap(err, "sbot/mount plug failed to make simple index")
			}
		}

		if mlm, ok := plug.(repo.MultiLogMaker); ok {
			err := MountMultiLog(plug.Name(), mlm.MakeMultiLog)(s)
			if err != nil {
				return errors.Wrap(err, "sbot/mount plug failed to make multilog")
			}
		}

		switch mode {
		case plugins2.AuthPublic:
			s.public.Register(plug)
		case plugins2.AuthMaster:
			s.master.Register(plug)
		case plugins2.AuthBoth:
			s.master.Register(plug)
			s.public.Register(plug)
		}
		return nil
	}
}

func MountMultiLog(name string, fn repo.MakeMultiLog) Option {
	return func(s *Sbot) error {
		mlog, serveFunc, err := fn(repo.New(s.repoPath))
		if err != nil {
			return errors.Wrapf(err, "sbot/index: failed to open idx %s", name)
		}
		s.closers.addCloser(mlog)
		s.serveIndex(s.rootCtx, name, serveFunc)
		s.mlogIndicies[name] = mlog
		return nil
	}
}

func MountSimpleIndex(name string, fn repo.MakeSimpleIndex) Option {
	return func(s *Sbot) error {
		idx, serveFunc, err := fn(repo.New(s.repoPath))
		if err != nil {
			return errors.Wrapf(err, "sbot/index: failed to open idx %s", name)
		}
		s.serveIndex(s.rootCtx, name, serveFunc)
		s.simpleIndex[name] = idx
		return nil
	}
}

func (s *Sbot) GetSimpleIndex(name string) (librarian.Index, bool) {
	si, has := s.simpleIndex[name]
	return si, has
}

func (s *Sbot) GetMultiLog(name string) (multilog.MultiLog, bool) {
	ml, has := s.mlogIndicies[name]
	return ml, has
}

func (s *Sbot) GetIndexNamesSimple() []string {
	var simple []string
	for name := range s.simpleIndex {
		simple = append(simple, name)
	}
	return simple
}

func (s *Sbot) GetIndexNamesMultiLog() []string {
	var mlogs []string
	for name := range s.mlogIndicies {
		mlogs = append(mlogs, name)
	}
	return mlogs
}

var _ ssb.Indexer = (*Sbot)(nil)

func (s *Sbot) serveIndex(ctx context.Context, name string, f repo.ServeFunc) {
	s.idxDone.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		err := f(ctx, s.RootLog, s.liveIndexUpdates)
		s.info.Log("event", "idx server exited", "idx", name, "error", err)
		if err != nil {
			err := s.Close()
			logging.CheckFatal(err)
			return
		}
	}(&s.idxDone)
}
