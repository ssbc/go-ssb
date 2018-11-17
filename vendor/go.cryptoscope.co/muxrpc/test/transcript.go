/*
This file is part of go-muxrpc.

go-muxrpc is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

go-muxrpc is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with go-muxrpc.  If not, see <http://www.gnu.org/licenses/>.
*/

package test

import (
	"io"
	"net"
	"sync"

	"github.com/hashicorp/go-multierror"

	"go.cryptoscope.co/muxrpc/codec"
)

type Direction bool

func (dir Direction) String() string {
	if dir == DirOut {
		return "DirOut"
	}

	return "DirIn"
}

const (
	DirOut Direction = false
	DirIn  Direction = true
)

type DirectedPacket struct {
	*codec.Packet
	Dir Direction
	Err error
}

type Transcript struct {
	l    sync.Mutex
	pkts []DirectedPacket
}

func (ts *Transcript) Append(dir Direction, pkt *codec.Packet) {
	ts.l.Lock()
	defer ts.l.Unlock()

	ts.pkts = append(ts.pkts, DirectedPacket{Dir: dir, Packet: pkt})
}

func (ts *Transcript) AppendError(dir Direction, err error) {
	ts.l.Lock()
	defer ts.l.Unlock()

	ts.pkts = append(ts.pkts, DirectedPacket{Dir: dir, Err: err})
}

func (ts *Transcript) Get() []DirectedPacket {
	return ts.pkts
}

func newTranscriptWriter(ts *Transcript, dir Direction) *transcriptWriter {
	r, w := io.Pipe()

	return &transcriptWriter{
		ts:          ts,
		dir:         dir,
		r:           codec.NewReader(r),
		WriteCloser: w,
	}
}

type transcriptWriter struct {
	ts  *Transcript
	dir Direction
	r   *codec.Reader
	io.WriteCloser
}

func (tw *transcriptWriter) work() (func(), chan error) {
	cancel := make(chan struct{})
	errCh := make(chan error)

	go func() {
		var (
			err error
			pkt *codec.Packet
		)

		defer func() {
			errCh <- err
		}()

		for {
			select {
			case <-cancel:
				return
			default:
			}

			pkt, err = tw.r.ReadPacket()
			if err != nil {
				tw.ts.AppendError(tw.dir, err)

				// don't send EOF over error channel, because that error is okay
				if err == io.EOF {
					err = nil
				}

				return
			}
			tw.ts.Append(tw.dir, pkt)
		}
	}()

	var closeOnce sync.Once

	return func() {
		closeOnce.Do(func() {
			close(cancel)
		})
	}, errCh
}

type transcriptWrapper struct {
	rwc io.ReadWriteCloser

	tOut io.Reader
	tIn  io.Reader
}

type closer func() error

func (c closer) Close() error { return c() }

// Wrap decodes every packet that passes through it and logs it
func Wrap(ts *Transcript, rwc io.ReadWriteCloser) io.ReadWriteCloser {
	twIn := newTranscriptWriter(ts, DirIn)
	cnclIn, errChIn := twIn.work()

	twOut := newTranscriptWriter(ts, DirOut)
	cnclOut, errChOut := twOut.work()

	return struct {
		io.Reader
		io.Writer
		io.Closer
	}{
		Reader: io.TeeReader(rwc, twIn),
		Writer: io.MultiWriter(rwc, twOut),
		Closer: closer(func() error {

			cnclIn()

			cnclOut()

			var err error

			err = multierror.Append(err, twIn.Close())

			err = multierror.Append(err, twOut.Close())

			if errIn := <-errChIn; errIn != io.EOF && errIn != nil {
				err = multierror.Append(err, errIn)
			}

			if errOut := <-errChOut; errOut != io.EOF && errOut != nil {
				err = multierror.Append(err, errOut)
			}

			err = multierror.Append(err, rwc.Close())

			// TODO: make better multierror -.-
			if merr, ok := err.(*multierror.Error); ok {
				if merr.Len() == 0 {
					err = nil
				}
			}

			return err
		}),
	}

	/*
		prout, pwout := io.Pipe()
		go func() {
			r := codec.NewReader(io.TeeReader(rwc, pwout))
			for {
				pkt, err := r.ReadPacket()
				if err != nil {
					ts.AppendError(DirIn, err)
					pwout.CloseWithError(err)
					return
				}
				ts.Append(DirIn, pkt)
			}
		}()

		prin, pwin := io.Pipe()
		go func() {
			r := codec.NewReader(io.TeeReader(prin, rwc))
			for {
				pkt, err := r.ReadPacket()
				if err != nil {
					ts.AppendError(DirOut, err)
					prin.CloseWithError(err)
					return
				}
				ts.Append(DirOut, pkt)
			}
		}()

		return struct {
			io.Reader
			io.Writer
			io.Closer
		}{Reader: prout, Writer: pwin, Closer: rwc}
	*/
}

type wrappedConn struct {
	net.Conn
	rwc io.ReadWriteCloser
}

func (conn *wrappedConn) Read(data []byte) (int, error) {
	return conn.rwc.Read(data)
}

func (conn *wrappedConn) Write(data []byte) (int, error) {
	return conn.rwc.Write(data)
}

func (conn *wrappedConn) Close() error {
	return conn.rwc.Close()
}

func WrapConn(ts *Transcript, conn net.Conn) net.Conn {
	return &wrappedConn{conn, Wrap(ts, conn)}
}
