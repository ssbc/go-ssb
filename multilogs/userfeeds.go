// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package multilogs

import (
	"context"
	"fmt"
	"sync"
	"time"

	refs "github.com/ssbc/go-ssb-refs"
	"github.com/ssbc/go-ssb/internal/storedrefs"
	"github.com/ssbc/margaret"
	"github.com/ssbc/margaret/multilog"
)

const IndexNameFeeds = "userFeeds"

var idxInSync sync.WaitGroup

func indexSyncStart() {
	idxInSync.Add(1)
}

func indexSyncDone() {
	time.AfterFunc(100 * time.Millisecond, func() {
		idxInSync.Done()
	})
}

// WaitUntilUserFeedIndexIsSynced blocks until all the index processing is in sync with the rootlog
func WaitUntilUserFeedIndexIsSynced() {
	idxInSync.Wait()
}

func UserFeedsUpdate(ctx context.Context, seq int64, value interface{}, mlog multilog.MultiLog) error {
	indexSyncStart()
	defer indexSyncDone()

	if nulled, ok := value.(error); ok {
		if margaret.IsErrNulled(nulled) {
			return nil
		}
		return nulled
	}

	abstractMsg, ok := value.(refs.Message)
	if !ok {
		return fmt.Errorf("error casting message. got type %T", value)
	}

	author := abstractMsg.Author()

	authorLog, err := mlog.Get(storedrefs.Feed(author))
	if err != nil {
		return fmt.Errorf("error opening sublog: %w", err)
	}

	_, err = authorLog.Append(seq)
	if err != nil {
		return fmt.Errorf("error appending new author message: %w", err)
	}
	return nil
}
