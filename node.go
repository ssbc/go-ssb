package ssb

import (
	"fmt"

	"github.com/pkg/errors"
)

var ErrShuttingDown = errors.Errorf("ssb: shutting down now") // this is fine

type ErrOutOfReach struct {
	Dist int
	Max  int
}

func (e ErrOutOfReach) Error() string {
	return fmt.Sprintf("ssb/graph: peer not in reach. d:%d, max:%d", e.Dist, e.Max)
}
