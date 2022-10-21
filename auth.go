// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ssb

import refs "github.com/ssbc/go-ssb-refs"

type Authorizer interface {
	Authorize(remote refs.FeedRef) error
}
