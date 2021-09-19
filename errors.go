// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"fmt"

	refs "go.mindeco.de/ssb-refs"
)

// ErrShuttingDown is returned if Shutdown() was canceld on an go-ssb server
var ErrShuttingDown = fmt.Errorf("ssb: shutting down now") // this is fine

// ErrWrongSequence is returned if there is a glitch on the current
// sequence number on the feed between in the offsetlog and the logical entry on the feed
type ErrWrongSequence struct {
	Ref             refs.FeedRef
	Logical, Stored int64
}

func (e ErrWrongSequence) Error() string {
	return fmt.Sprintf("ssb/consistency error: message sequence missmatch for feed %s Stored:%d Logical:%d",
		e.Ref.String(),
		e.Stored,
		e.Logical)
}
