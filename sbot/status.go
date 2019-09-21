package sbot

import (
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/netwrap"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/multiserver"
)

func (sbot *Sbot) Status() (*ssb.Status, error) {
	v, err := sbot.RootLog.Seq().Value()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get root log sequence")
	}

	s := ssb.Status{
		Root:  v.(margaret.Seq),
		Blobs: sbot.WantManager.AllWants(),
	}
	mlogNames := sbot.GetIndexNamesMultiLog()
	s.Indexes.MultiLog = make(map[string]int64, len(mlogNames))

	for _, name := range mlogNames {

		mlog, has := sbot.GetMultiLog(name)
		if !has {
			continue
		}

		if qspec, has := mlog.(interface{ QuerySpec() margaret.QuerySpec }); has {
			var gc getCurrent
			err := qspec.QuerySpec()(&gc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get index state of %s", name)
			}
			s.Indexes.MultiLog[name] = gc.seq
		} else {
			// fmt.Printf("DEBUG/mlog(%s) type:%T\n", name, mlog)
			s.Indexes.MultiLog[name] = -2
		}
	}

	simpleNames := sbot.GetIndexNamesSimple()
	s.Indexes.Simple = make(map[string]int64, len(simpleNames))
	for _, name := range simpleNames {

		idx, has := sbot.GetSimpleIndex(name)
		if !has {
			continue
		}

		if qspec, has := idx.(interface{ QuerySpec() margaret.QuerySpec }); has {
			var gc getCurrent
			err := qspec.QuerySpec()(&gc)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get index state of %s", name)
			}

			s.Indexes.Simple[name] = gc.seq
		} else {
			// fmt.Printf("DEBUG/simple(%s) type:%T\n", name, idx)
			s.Indexes.Simple[name] = -2
		}

	}

	edps := sbot.Network.GetAllEndpoints()

	sort.Sort(byConnTime(edps))

	for _, es := range edps {
		var ms multiserver.NetAddress
		ms.Ref = es.ID
		if tcpAddr, ok := netwrap.GetAddr(es.Addr, "tcp").(*net.TCPAddr); ok {
			ms.Addr = *tcpAddr
		}
		s.Peers = append(s.Peers, ssb.PeerStatus{
			Addr:  ms.String(),
			Since: humanize.Time(time.Now().Add(-es.Since)),
		})
	}
	return &s, nil
}

type byConnTime []ssb.EndpointStat

func (bct byConnTime) Len() int {
	return len(bct)
}

func (bct byConnTime) Less(i int, j int) bool {
	return bct[i].Since < bct[j].Since
}

func (bct byConnTime) Swap(i int, j int) {
	bct[i], bct[j] = bct[j], bct[i]
}

type getCurrent struct {
	seq int64
}

func (qry *getCurrent) Gt(s margaret.Seq) error {
	qry.seq = s.Seq()
	return nil
}

func (qry *getCurrent) Gte(s margaret.Seq) error {
	qry.seq = s.Seq() + 1
	return nil
}

func (qry *getCurrent) Lt(s margaret.Seq) error  { return nil }
func (qry *getCurrent) Lte(s margaret.Seq) error { return nil }
func (qry *getCurrent) Limit(n int) error        { return nil }
func (qry *getCurrent) Live(live bool) error     { return nil }
func (qry *getCurrent) SeqWrap(wrap bool) error  { return nil }
func (qry *getCurrent) Reverse(yes bool) error   { return nil }

func (s *Sbot) FSCK(idxlog multilog.MultiLog) {

	checkFatal := func(err error) {
		if err != nil {
			level.Error(s.info).Log("error", "fsck failed", "err", err)
			// s.Shutdown()
			// s.Close()
			// os.Exit(1)

			return
		}
	}

	if idxlog == nil {
		var ok bool
		idxlog, ok = s.GetMultiLog("userFeeds")
		if !ok {
			checkFatal(errors.Errorf("no multilog"))
			return
		}
	}

	// gb := s.GraphBuilder

	feeds, err := idxlog.List()
	checkFatal(err)

	// var followCnt, msgCount uint
	for _, author := range feeds {
		var sr ssb.StorageRef
		err := sr.Unmarshal([]byte(author))
		checkFatal(err)
		authorRef, err := sr.FeedRef()
		checkFatal(err)

		subLog, err := idxlog.Get(author)
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

		// msgCount += uint(msg.Seq())

		// f, err := gb.Follows(authorRef)
		// checkFatal(err)

		// if len(feeds) < 20 {
		// 	h := gb.Hops(authorRef, 2)
		// 	level.Info(s.info).Log("feed", authorRef.Ref(), "seq", msg.Seq(), "follows", f.Count(), "hops", h.Count())
		// }
		// followCnt += uint(f.Count())
	}
}
