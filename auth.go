// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import (
	"fmt"

	refs "go.mindeco.de/ssb-refs"
)

// Authorizer returns nil if a remote should be authorized by the server
// should return ErrOutOfReach if applies.
type Authorizer interface {
	Authorize(remote refs.FeedRef) error
}

// ErrOutOfReach returns the distance to another remote peer on the follow graph
type ErrOutOfReach struct {
	Dist int
	Max  int
}

func (e ErrOutOfReach) Error() string {
	return fmt.Sprintf("ssb/graph: peer not in reach. d:%d, max:%d", e.Dist, e.Max)
}
