package debug

import (
	"io"
	"net"
	"os"
	"path/filepath"

	"github.com/cryptix/go/logging"
)

// Dump decodes every packet that passes through it and logs it
// TODO: add timestmaps to individual packets
// TODO: maybe make it one file - something like pcap would be rat but it's also quite the beast
func Dump(path string, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	os.MkdirAll(path, 0700)
	rx, err := os.Create(filepath.Join(path, "rx"))
	logging.CheckFatal(err)
	tx, err := os.Create(filepath.Join(path, "tx"))
	logging.CheckFatal(err)
	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: io.TeeReader(rwc, rx),
		Writer: io.MultiWriter(rwc, tx),
		Closer: closer(func() error {
			rx.Close()
			tx.Close()
			return rwc.Close()
		}),
	}

}

func WrapDump(path string, c net.Conn) (net.Conn, error) {
	return &wrappedConn{c, Dump(path, c)}, nil
}
