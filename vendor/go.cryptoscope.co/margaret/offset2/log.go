package offset2 // import "go.cryptoscope.co/margaret/offset2"

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

// DefaultFrameSize is the default frame size.
const DefaultFrameSize = 4096

type offsetLog struct {
	l    sync.Mutex
	name string

	jrnl *journal
	ofst *offset
	data *data

	seq   luigi.Observable
	codec margaret.Codec

	bcast  luigi.Broadcast
	bcSink luigi.Sink
}

func (log *offsetLog) Close() error {
	// TODO: close open querys?
	// log.l.Lock()
	// defer log.l.Unlock()

	if err := log.jrnl.Close(); err != nil {
		return errors.Wrap(err, "journal file close failed")
	}

	if err := log.ofst.Close(); err != nil {
		return errors.Wrap(err, "offset file close failed")
	}

	if err := log.data.Close(); err != nil {
		return errors.Wrap(err, "data file close failed")
	}

	if err := log.bcSink.Close(); err != nil {
		return errors.Wrap(err, "log broadcast close failed")
	}

	return nil
}

// Null overwrites the entry at seq with zeros
// updating is kinda odd in append-only
// but in some cases you still might want to redact entries
func (log *offsetLog) Null(seq margaret.Seq) error {
	log.bcSink.Close()

	log.l.Lock()
	defer log.l.Unlock()

	ofst, err := log.ofst.readOffset(seq)
	if err != nil {
		return errors.Wrap(err, "error read offset")
	}

	sz, err := log.data.getFrameSize(ofst)
	if err != nil {
		return errors.Wrap(err, "get frame size failed")
	}

	var minusSz bytes.Buffer
	err = binary.Write(&minusSz, binary.BigEndian, -sz)
	if err != nil {
		return errors.Wrapf(err, "failed to encode neg size: %d", -sz)
	}

	_, err = log.data.WriteAt(minusSz.Bytes(), ofst)
	if err != nil {
		return errors.Wrapf(err, "failed to write -1 size bytes at %d", ofst)
	}

	nulls := make([]byte, sz)
	_, err = log.data.WriteAt(nulls, ofst+8)
	if err != nil {
		return errors.Wrapf(err, "failed to write %d bytes at %d", sz, ofst)
	}

	return nil
}

// Open returns a the offset log in the directory at `name`.
// If it is empty or does not exist, a new log will be created.
func Open(name string, cdc margaret.Codec) (margaret.Log, error) {
	err := os.MkdirAll(name, 0700)
	if err != nil {
		return nil, errors.Wrapf(err, "error making log directory at %q", name)
	}

	pLog := filepath.Join(name, "data")
	fData, err := os.OpenFile(pLog, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening log data file at %q", pLog)
	}

	pOfst := filepath.Join(name, "ofst")
	fOfst, err := os.OpenFile(pOfst, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening log offset file at %q", pOfst)
	}

	pJrnl := filepath.Join(name, "jrnl")
	fJrnl, err := os.OpenFile(pJrnl, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening log journal file at %q", pJrnl)
	}

	log := &offsetLog{
		name: name,

		jrnl: &journal{fJrnl},
		ofst: &offset{fOfst},
		data: &data{File: fData},

		codec: cdc,
	}

	err = log.checkJournal()
	if err != nil {
		return nil, errors.Wrap(err, "integrity error")
	}

	log.bcSink, log.bcast = luigi.NewBroadcast()

	// get current sequence by end / blocksize
	end, err := fOfst.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to seek to end of log-offset-file")
	}
	// assumes -1 is SeqEmpty
	log.seq = luigi.NewObservable(margaret.BaseSeq((end / 8) - 1))

	return log, nil
}

// checkJournal verifies that the last entry is consistent along the three files.
//  - read sequence from journal
//  - read last offset from offset file
//  - read frame size from data file at previously read offset
//  - check that the end of the frame is also end of file
//  - check that number of entries in offset file equals value in journal
func (log *offsetLog) checkJournal() error {
	seqJrnl, err := log.jrnl.readSeq()
	if err != nil {
		return errors.Wrap(err, "error reading seq")
	}

	if seqJrnl.Seq() == -1 {
		statOfst, err := log.ofst.Stat()
		if err != nil {
			return errors.Wrap(err, "stat failed on offset file")
		}

		if statOfst.Size() != 0 {
			return errors.New("journal empty but offset file isnt")
		}

		statData, err := log.data.Stat()
		if err != nil {
			return errors.Wrap(err, "stat failed on data file")
		}

		if statData.Size() != 0 {
			return errors.New("journal empty but data file isnt")
		}

		return nil
	}

	ofstData, seqOfst, err := log.ofst.readLastOffset()
	if err != nil {
		return errors.Wrap(err, "error reading last entry of log offset file")
	}

	sz, err := log.data.getFrameSize(ofstData)
	if err != nil {
		return errors.Wrap(err, "error getting frame size from log data file")
	}

	if sz < 0 { // entry nulled
		// irrelevant here, just treat the nulls as regular bytes
		sz = -sz
	}

	stat, err := log.data.Stat()
	if err != nil {
		return errors.Wrap(err, "error stat'ing data file")
	}

	n := ofstData + 8 + sz
	d := n - stat.Size()
	if d != 0 {
		// TODO: chop off the rest
		return errors.Errorf("data file size difference %d", d)
	}

	if seqJrnl != seqOfst {
		return errors.Errorf("seq in journal does not match element count in log offset file - %d != %d", seqJrnl, seqOfst)
	}

	return nil
}

// checkConsistency is an fsck for the offset log.
func (log *offsetLog) checkConsistency() error {
	err := log.checkJournal()
	if err != nil {
		return errors.Wrap(err, "journal inconsistent")
	}

	var (
		ofst, nextOfst int64
		seq            margaret.BaseSeq
	)

	for {
		sz, err := log.data.getFrameSize(nextOfst)
		if errors.Cause(err) == io.EOF {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "error getting frame size")
		}

		ofst = nextOfst

		nextOfst += sz + 8 // 8 byte length prefix

		expOfst, err := log.ofst.readOffset(seq)
		if err != nil {
			return errors.Wrap(err, "error reading expected offset")
		}

		if ofst != expOfst {
			return errors.Errorf("offset mismatch: offset file says %d, data file has %d", expOfst, ofst)
		}
	}
}

func (log *offsetLog) Seq() luigi.Observable {
	return log.seq
}

func (log *offsetLog) Get(seq margaret.Seq) (interface{}, error) {
	log.l.Lock()
	defer log.l.Unlock()

	v, err := log.readFrame(seq)
	if errors.Cause(err) == io.EOF {
		return v, luigi.EOS{}
	}

	return v, err
}

// readFrame reads and parses a frame.
func (log *offsetLog) readFrame(seq margaret.Seq) (interface{}, error) {
	ofst, err := log.ofst.readOffset(seq)
	if err != nil {
		return nil, errors.Wrap(err, "error read offset")
	}

	r, err := log.data.frameReader(ofst)
	if err != nil {
		return nil, errors.Wrap(err, "error getting frame reader")
	}

	dec := log.codec.NewDecoder(r)
	v, err := dec.Decode()
	if errors.Cause(err) == io.EOF {
		return v, luigi.EOS{}
	}

	return v, err
}

func (log *offsetLog) Query(specs ...margaret.QuerySpec) (luigi.Source, error) {
	log.l.Lock()
	defer log.l.Unlock()

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

	log.l.Lock()
	defer log.l.Unlock()

	jrnlSeq, err := log.jrnl.bump()
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error bumping journal")
	}

	ofst, err := log.data.append(data)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error appending data")
	}

	seq, err := log.ofst.append(ofst)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error appending offset")
	}

	if seq != jrnlSeq {
		return margaret.SeqEmpty, errors.Errorf("seq mismatch: journal wants %d, offset has %d", jrnlSeq, seq)
	}

	err = log.bcSink.Pour(context.TODO(), margaret.WrapWithSeq(v, jrnlSeq))
	log.seq.Set(jrnlSeq)

	return seq, nil
}

func (log *offsetLog) FileName() string {
	return log.name
}
