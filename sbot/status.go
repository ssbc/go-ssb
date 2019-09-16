package sbot

import (
	"fmt"
	"os"

	"github.com/go-kit/kit/log/level"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

func (s *Sbot) FSCK() {

	checkFatal := func(err error) {
		if err != nil {
			level.Error(s.info).Log("error", "fsck failed", "err", err)
			s.Shutdown()
			s.Close()
			os.Exit(1)

			return
		}
	}
	uf, ok := s.GetMultiLog("userFeeds")
	if !ok {
		checkFatal(fmt.Errorf("missing userFeeds"))
		return
	}

	gb := s.GraphBuilder

	feeds, err := uf.List()
	checkFatal(err)

	var followCnt, msgCount uint
	for _, author := range feeds {
		authorRef, err := ssb.ParseFeedRef(string(author))
		checkFatal(err)

		subLog, err := uf.Get(authorRef.StoredAddr())
		checkFatal(err)

		userLogV, err := subLog.Seq().Value()
		checkFatal(err)
		userLogSeq := userLogV.(margaret.Seq)
		rlSeq, err := subLog.Get(userLogSeq)
		if margaret.IsErrNulled(err) {
			continue
		} else {
			checkFatal(err)
		}
		rv, err := s.RootLog.Get(rlSeq.(margaret.BaseSeq))
		if margaret.IsErrNulled(err) {
			continue
		} else {
			checkFatal(err)
		}
		msg := rv.(ssb.Message)

		if msg.Seq() != userLogSeq.Seq()+1 {
			err = fmt.Errorf("light fsck failed: head of feed mismatch on %s: %d vs %d", authorRef.Ref(), msg.Seq(), userLogSeq.Seq()+1)
			checkFatal(err)
		}

		msgCount += uint(msg.Seq())

		f, err := gb.Follows(authorRef)
		checkFatal(err)

		if len(feeds) < 20 {
			h := gb.Hops(authorRef, 2)
			level.Info(s.info).Log("feed", authorRef.Ref(), "seq", msg.Seq(), "follows", f.Count(), "hops", h.Count())
		}
		followCnt += uint(f.Count())
	}
}
