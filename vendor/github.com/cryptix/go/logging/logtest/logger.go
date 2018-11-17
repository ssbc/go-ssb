package logtest

import (
	"bufio"
	"io"
	"testing"

	"github.com/go-kit/kit/log"
)

// Logger logs every line it is written to it. t.Log(prefix: line)
func Logger(prefix string, t *testing.T) io.WriteCloser {
	pr, pw := io.Pipe()
	go func() {
		s := bufio.NewScanner(pr)
		for s.Scan() {
			t.Logf("%s: %q", prefix, s.Text())
		}
		if err := s.Err(); err != nil {
			t.Errorf("%s: scanner error:%s", prefix, err)
		}
	}()
	return pw
}

func KitLogger(test string, t testing.TB) (log.Logger, io.Writer) {
	pr, pw := io.Pipe()
	go func() {
		s := bufio.NewScanner(pr)
		for s.Scan() {
			t.Logf(s.Text())
		}
		if err := s.Err(); err != nil {
			t.Errorf("%s: scanner error:%s", test, err)
		}
	}()
	logger := log.NewLogfmtLogger(log.NewSyncWriter(pw))
	logger = log.With(logger, "test", test)
	return logger, pw
}
