package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	stderr "errors"
	"io"
	"sync"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"

	"github.com/pkg/errors"
)

// Packer is a duplex stream that sends and receives *codec.Packet values.
// Usually wraps a network connection or stdio.
type Packer interface {
	luigi.Source
	luigi.Sink
}

// NewPacker takes an io.ReadWriteCloser and returns a Packer.
func NewPacker(rwc io.ReadWriteCloser) Packer {
	return &packer{
		r: codec.NewReader(rwc),
		w: codec.NewWriter(rwc),
		c: rwc,

		closing: make(chan struct{}),
	}
}

// packer wraps an io.ReadWriteCloser and implements Packer.
type packer struct {
	rl sync.Mutex
	wl sync.Mutex

	r *codec.Reader
	w *codec.Writer
	c io.Closer

	closeOnce sync.Once
	closing   chan struct{}
}

// Next returns the next packet from the underlying stream.
func (pkr *packer) Next(ctx context.Context) (interface{}, error) {
	pkr.rl.Lock()
	defer pkr.rl.Unlock()

	pkt, err := pkr.r.ReadPacket()
	select {
	case <-pkr.closing:
		if err != nil {
			return nil, luigi.EOS{}
		}
	default:
	}

	if errors.Cause(err) == io.EOF {
		if err = pkr.Close(); err != nil {
			return nil, errors.Wrap(err, "error closing connection after reading EOF")
		}

		return nil, luigi.EOS{}
	} else if err != nil {
		return nil, errors.Wrap(err, "error reading packet")
	}

	pkt.Req = -pkt.Req

	return pkt, nil
}

// Pour sends a packet to the underlying stream.
func (pkr *packer) Pour(ctx context.Context, v interface{}) error {
	pkt, ok := v.(*codec.Packet)
	if !ok {
		return errors.Errorf("packer sink expected type *codec.Packet, got %T", v)
	}

	pkr.wl.Lock()
	defer pkr.wl.Unlock()
	err := pkr.w.WritePacket(pkt)
	if err != nil {
		select {
		case <-pkr.closing:
			err = errSinkClosed
		default:
		}
	}

	return errors.Wrap(err, "muxrpc: error writing packet")
}

var errSinkClosed = stderr.New("muxrpc: pour to closed sink")

// IsSinkClosed should be moved to luigi to gether with the error
func IsSinkClosed(err error) bool {
	if err := errors.Cause(err); err == errSinkClosed {
		return true
	}
	return false
}

// Close closes the packer.
func (pkr *packer) Close() error {
	var err error

	pkr.closeOnce.Do(func() {
		close(pkr.closing)
		err = pkr.c.Close()
	})

	return errors.Wrap(err, "error closing underlying closer")
}
