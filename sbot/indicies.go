package sbot

import (
	"context"
	"os"
	"sync"

	"github.com/cryptix/go/logging"
	"github.com/pkg/errors"
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
		if serveFunc != nil {
			s.serveIndex(s.rootCtx, name, serveFunc)
		}
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

func (s *Sbot) GetMultiLog(name string) (multilog.MultiLog, bool) {
	ml, has := s.mlogIndicies[name]
	return ml, has
}

func (s *Sbot) serveIndex(ctx context.Context, name string, f repo.ServeFunc) {
	s.idxDone.Add(1)
	go func(wg *sync.WaitGroup) {
		err := f(ctx, s.RootLog, s.liveIndexUpdates)
		s.info.Log("event", "idx server exited", "idx", name, "error", err)
		if err != nil {
			err := s.Close()
			logging.CheckFatal(err)
			os.Exit(1)
			return
		}
		wg.Done()
	}(&s.idxDone)
}
