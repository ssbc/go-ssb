package testutils

import (
	"io"
	"os"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
)

func NewRelativeTimeLogger(w io.Writer) log.Logger {
	if w == nil {
		w = os.Stderr
	}

	var rtl relTimeLogger
	rtl.start = time.Now()

	mainLog := log.NewLogfmtLogger(w)
	return log.With(mainLog, "t", log.Valuer(rtl.diffTime))
}

type relTimeLogger struct {
	sync.Mutex

	start time.Time
}

func (rtl *relTimeLogger) diffTime() interface{} {
	rtl.Lock()
	defer rtl.Unlock()
	newStart := time.Now()
	since := newStart.Sub(rtl.start)
	return since
}
