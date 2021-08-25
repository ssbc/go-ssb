//go:build none
// +build none

package p

import (
	refs "go.mindeco.de/ssb-refs"
)

func before(r refs.BlobRef) string {
	return r.ShortRef()
}

func after(r refs.BlobRef) string {
	return r.ShortSigil()
}
