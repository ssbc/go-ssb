package sbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/multilogs"
)

// FSCKMode is an enum for the sbot.FSCK function
type FSCKMode uint

const (
	// FSCKModeLength just checks the feed lengths
	FSCKModeLength FSCKMode = iota

	// FSCKModeSequences makes sure the sequence field of each message on a feed are increasing correctly
	FSCKModeSequences

	// FSCKModeVerify does a full signature and hash verification
	// FSCKModeVerify
)

// FSCK checks the consistency of the received messages and the indexes
func (s *Sbot) FSCK(feedsMlog multilog.MultiLog, mode FSCKMode) error {
	if feedsMlog == nil {
		var ok bool
		feedsMlog, ok = s.GetMultiLog(multilogs.IndexNameFeeds)
		if !ok {
			return errors.New("sbot: no users multilog")
		}
	}

	switch mode {
	case FSCKModeLength:
		return lengthFSCK(feedsMlog, s.RootLog)

	case FSCKModeSequences:
		return sequenceFSCK(s.RootLog)

	default:
		return errors.New("sbot: unknown fsck mode")
	}
}

// lengthFSCK just checks the length of each stored feed.
// It expects a multilog as first parameter where each sublog is one feed
// and each entry maps to another entry in the receiveLog
func lengthFSCK(authorMlog multilog.MultiLog, receiveLog margaret.Log) error {
	feeds, err := authorMlog.List()
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

		subLog, err := authorMlog.Get(author)
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

		rv, err := receiveLog.Get(rlSeq.(margaret.BaseSeq))
		if err != nil {
			if margaret.IsErrNulled(err) {
				continue
			}
			return err
		}
		msg := rv.(ssb.Message)

		// margaret indexes are 0-based, therefore +1
		if msg.Seq() != currentSeqFromIndex.Seq()+1 {
			return ssb.ErrWrongSequence{
				Ref:     authorRef,
				Stored:  currentSeqFromIndex,
				Logical: msg,
			}
		}
	}

	return nil
}

// sequenceFSCK goes through every message in the receiveLog
// and checks tha the sequence of a feed is correctly increasing by one each message
func sequenceFSCK(receiveLog margaret.Log) error {
	ctx := context.Background()

	lastSequence := make(map[string]int64)

	currentSeqV, err := receiveLog.Seq().Value()
	if err != nil {
		return err
	}
	currentOffsetSeq := currentSeqV.(margaret.Seq).Seq()
	start := time.Now()

	src, err := receiveLog.Query(margaret.SeqWrap(true))
	if err != nil {
		return err
	}

	// which feeds have problems
	var consistencyErrors []ssb.ErrWrongSequence

	// all the sequences of broken messages
	var brokenSequences []int64

	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			return err
		}

		sw, ok := v.(margaret.SeqWrapper)
		if !ok {
			if errv, ok := v.(error); ok && margaret.IsErrNulled(errv) {
				continue
			}
			return fmt.Errorf("fsck/sw: unexpected message type: %T (wanted %T)", v, sw)
		}

		rxLogSeq := sw.Seq().Seq()
		val := sw.Value()
		msg, ok := val.(ssb.Message)
		if !ok {
			return fmt.Errorf("fsck/value: unexpected message type: %T (wanted %T)", val, msg)
		}

		msgSeq := msg.Seq()
		authorRef := msg.Author().Ref()
		currSeq, has := lastSequence[authorRef]

		// TODO: unify these checks
		if !has {
			if msgSeq != 1 {
				seqErr := ssb.ErrWrongSequence{
					Ref:     msg.Author(),
					Stored:  margaret.SeqEmpty,
					Logical: msg,
				}
				consistencyErrors = append(consistencyErrors, seqErr)
				lastSequence[authorRef] = -1
				brokenSequences = append(brokenSequences, rxLogSeq)
				continue
			}
			lastSequence[authorRef] = 1
			continue
		}

		if currSeq < 0 { // feed broken, skipping
			brokenSequences = append(brokenSequences, rxLogSeq)
			continue
		}

		if currSeq+1 != msgSeq {
			seqErr := ssb.ErrWrongSequence{
				Ref:     msg.Author(),
				Stored:  margaret.BaseSeq(currSeq + 1),
				Logical: msg,
			}
			consistencyErrors = append(consistencyErrors, seqErr)
			lastSequence[authorRef] = -1
			brokenSequences = append(brokenSequences, rxLogSeq)
			continue
		}
		lastSequence[authorRef] = currSeq + 1

		// bench stats
		currentOffsetSeq--
		if time.Since(start) > time.Second {
			fmt.Println("fsck/sequence: messages left to process:", currentOffsetSeq)
			start = time.Now()
		}
	}

	if len(consistencyErrors) == 0 && len(brokenSequences) == 0 {
		return nil
	}

	// error report
	return ssb.ErrConsistencyProblems{
		Errors:    consistencyErrors,
		Sequences: brokenSequences,
	}
}
