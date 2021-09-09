//go:build none
// +build none

package p

import (
	refs "go.mindeco.de/ssb-refs"
)

func before(r refs.BlobRef) string {
	return r.Ref()
}

func after(r refs.BlobRef) string {
	return r.Sigil()
}
