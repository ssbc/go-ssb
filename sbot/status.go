// SPDX-License-Identifier: MIT

package sbot

import (
	"fmt"
	"net"
	"os"
	"sort"
	"time"

	"github.com/dustin/go-humanize"
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

func (s *Sbot) FSCK(idxlog multilog.MultiLog) error {
	if idxlog == nil {
		var ok bool
		idxlog, ok = s.GetMultiLog(multilogs.IndexNameFeeds)
		if !ok {
			return errors.Errorf("sbot: no users multilog")
		}
	}

	feeds, err := idxlog.List()
	if err != nil {
		return err
	}

	for _, author := range feeds {
		var sr ssb.StorageRef
		err := sr.Unmarshal([]byte(author))
		if err != nil {
			return err
		}
		authorRef, err := sr.FeedRef()
		if err != nil {
			return err
		}

		subLog, err := idxlog.Get(author)
		if err != nil {
			return err
		}

		currentSeqV, err := subLog.Seq().Value()
		if err != nil {
			return err
		}
		currentSeqFromIndex := currentSeqV.(margaret.Seq)
		rlSeq, err := subLog.Get(currentSeqFromIndex)
		if err != nil {
			if margaret.IsErrNulled(err) {
				continue
			}
			return err
		}

		rv, err := s.RootLog.Get(rlSeq.(margaret.BaseSeq))
		if err != nil {
			if margaret.IsErrNulled(err) {
				continue
			}
			return err
		}
		msg := rv.(ssb.Message)

		if msg.Seq() != currentSeqFromIndex.Seq()+1 {
			err = fmt.Errorf("light fsck failed: head of feed mismatch on %s: %d vs %d",
				authorRef.Ref(),
				msg.Seq(),
				currentSeqFromIndex.Seq()+1)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
