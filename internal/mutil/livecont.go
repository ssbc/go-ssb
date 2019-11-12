package mutil

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type LiveResumer struct {
	l    sync.Mutex
	sink luigi.Sink

	seq int64
	f   *os.File
	buf [8]byte
}

func NewLiveResumer(f *os.File, sink luigi.Sink) (lr *LiveResumer, err error) {
	lr = &LiveResumer{
		sink: sink,
		f:    f,
	}

	lr.seq, err = lr.getSeq()

	return
}

func (lr *LiveResumer) Pour(ctx context.Context, v interface{}) error {
	lr.l.Lock()
	defer lr.l.Unlock()

	seqwrap, ok := v.(margaret.SeqWrapper)
	if !ok {
		return errors.New("expecting seqwrapped value")
	}

	seq := seqwrap.Seq()

	err := lr.sink.Pour(ctx, seqwrap.Value())
	if err != nil {
		return errors.Wrap(err, "error pouring value")
	}

	err = lr.setSeq(seq.Seq())
	if err != nil {
		return errors.Wrap(err, "error setting new seq")
	}

	lr.seq = seq.Seq()

	return nil
}

func (lr *LiveResumer) getSeq() (int64, error) {
	_, err := lr.f.ReadAt(lr.buf[:], 0)
	if err == io.EOF {
		return int64(margaret.SeqEmpty), nil
	} else if err != nil {
		return 0, errors.Wrap(err, "error reading seq")
	}

	return int64(binary.LittleEndian.Uint64(lr.buf[:])), nil
}

func (lr *LiveResumer) setSeq(seq int64) error {
	binary.LittleEndian.PutUint64(lr.buf[:], uint64(seq))
	_, err := lr.f.WriteAt(lr.buf[:], 0)
	return errors.Wrap(err, "error writing seq")
}

func (lr *LiveResumer) QuerySpec() margaret.QuerySpec {
	return margaret.MergeQuerySpec(
		margaret.Gt(margaret.BaseSeq(lr.seq)),
		margaret.SeqWrap(true))
}
