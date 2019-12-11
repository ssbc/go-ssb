// SPDX-License-Identifier: MIT

package sbot

import (
	"fmt"
	"net"
	"os"
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
	"go.cryptoscope.co/ssb/multilogs"
)

func (sbot *Sbot) Status() (ssb.Status, error) {
	v, err := sbot.RootLog.Seq().Value()
	if err != nil {
		return ssb.Status{}, errors.Wrap(err, "failed to get root log sequence")
	}

	s := ssb.Status{
		PID:   os.Getpid(),
		Root:  v.(margaret.Seq),
		Blobs: sbot.WantManager.AllWants(),
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
	return s, nil
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
		idxlog, ok = s.GetMultiLog(multilogs.IndexNameFeeds)
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
	}
}
