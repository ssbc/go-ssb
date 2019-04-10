package offset // import "go.cryptoscope.co/margaret/offset"

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

// DefaultFrameSize is the default frame size.
const DefaultFrameSize = 4096

type offsetLog struct {
	l sync.Mutex
	f *os.File

	seq     luigi.Observable
	codec   margaret.Codec
	framing margaret.Framing

	bcast  luigi.Broadcast
	bcSink luigi.Sink
}

// New returns a new offset log.
func New(f *os.File, framing margaret.Framing, cdc margaret.Codec) (margaret.Log, error) {
	log := &offsetLog{
		f:       f,
		framing: framing,
		codec:   cdc,
	}

	log.bcSink, log.bcast = luigi.NewBroadcast()

	// get current sequence by end / blocksize
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to seek to end of log-file")
	}
	// assumes -1 is SeqEmpty
	log.seq = luigi.NewObservable(margaret.BaseSeq((end / framing.FrameSize()) - 1))

	return log, nil
}

func (log *offsetLog) Close() error {
	log.l.Lock()
	defer log.l.Unlock()
	if log.f == nil {
		return io.ErrClosedPipe // already closed
	}

	// log.bcSink.Pour() // errClosed?
	if err := log.bcSink.Close(); err != nil {
		return errors.Wrap(err, "offsetLog failed to close broadcast")
	}

	if err := log.f.Close(); err != nil {
		return errors.Wrap(err, "offsetLog failed to close backing file")
	}
	log.f = nil

	return nil
}

func (log *offsetLog) Seq() luigi.Observable {
	return log.seq
}

func (log *offsetLog) Get(seq margaret.Seq) (interface{}, error) {
	log.l.Lock()
	defer log.l.Unlock()
	if log.f == nil {
		return nil, io.ErrClosedPipe // already closed
	}

	return log.readFrame(seq)
}

// readFrame reads and parses a frame.
func (log *offsetLog) readFrame(seq margaret.Seq) (interface{}, error) {
	frame := make([]byte, log.framing.FrameSize())
	pos := seq.Seq() * log.framing.FrameSize()

	n, err := log.f.ReadAt(frame, pos)
	if err != nil {
		return nil, errors.Wrap(err, "error reading frame")
	}

	if int64(n) != log.framing.FrameSize() {
		return nil, errors.New("short read")
	}

	data, err := log.framing.DecodeFrame(frame)
	if err != nil {
		return nil, errors.Wrap(err, "error decoding frame")
	}

	v, err := log.codec.Unmarshal(data)
	if err != nil {
		return nil, errors.Wrap(err, "error unmarshaling data")
	}

	return v, nil
}

func (log *offsetLog) Query(specs ...margaret.QuerySpec) (luigi.Source, error) {
	log.l.Lock()
	defer log.l.Unlock()
	if log.f == nil {
		return nil, io.ErrClosedPipe // already closed
	}

	qry := &offsetQuery{
		log:   log,
		codec: log.codec,

		nextSeq: margaret.SeqEmpty,
		lt:      margaret.SeqEmpty,

		limit: -1, //i.e. no limit
		close: make(chan struct{}),
	}

	for _, spec := range specs {
		err := spec(qry)
		if err != nil {
			return nil, err
		}
	}

	return qry, nil
}

func (log *offsetLog) Append(v interface{}) (margaret.Seq, error) {
	data, err := log.codec.Marshal(v)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error marshaling value")
	}

	var nextSeq margaret.BaseSeq

	log.l.Lock()
	defer log.l.Unlock()
	if log.f == nil {
		return nil, io.ErrClosedPipe // already closed
	}

	frame, err := log.framing.EncodeFrame(data)
	if err != nil {
		return margaret.SeqEmpty, err
	}

	_, err = log.f.Write(frame)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error writng frame")
	}

	currSeq, err := log.seq.Value()
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error reading current sequence number")
	}

	nextSeq = margaret.BaseSeq(currSeq.(margaret.Seq).Seq()) + 1

	err = log.bcSink.Pour(context.TODO(), margaret.WrapWithSeq(v, nextSeq))

	// we need to do this, even in case of errors.
	// but it should happen after the processing finished.
	log.seq.Set(nextSeq)

	return nextSeq, errors.Wrap(err, "error in live log queries")
}

func (log *offsetLog) FileName() string {
	return log.f.Name()
}
