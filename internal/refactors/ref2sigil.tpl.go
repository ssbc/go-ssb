// SPDX-FileCopyrightText: 2023 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

//go:build none
// +build none

package p

import (
	refs "github.com/ssbc/go-ssb-refs"
)

func before(r refs.MessageRef) string {
	return r.Sigil()
}

func after(r refs.MessageRef) string {
	return r.String()
}
