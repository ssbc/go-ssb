package offset2 // import "go.cryptoscope.co/margaret/offset2"

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"path"
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

// Open returns a the offset log in the directory at `name`.
// If it is empty or does not exist, a new log will be created.
func Open(name string, cdc margaret.Codec) (margaret.Log, error) {
	err := os.MkdirAll(name, 0700)
	if err != nil {
		return nil, errors.Wrapf(err, "error making log directory at %q", name)
	}

	pLog := path.Join(name, "data")
	fData, err := os.OpenFile(pLog, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening log data file at %q", pLog)
	}

	pOfst := path.Join(name, "ofst")
	fOfst, err := os.OpenFile(pOfst, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.Wrapf(err, "error opening log offset file at %q", pOfst)
	}

	pJrnl := path.Join(name, "jrnl")
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

		statData, err := log.ofst.Stat()
		if err != nil {
			return errors.Wrap(err, "stat failed on offset file")
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

	stat, err := log.data.Stat()
	if err != nil {
		return errors.Wrap(err, "error stat'ing data file")
	}

	if ofstData+8+sz > stat.Size() {
		return errors.New("data file is too short")
	}
	if ofstData+8+sz < stat.Size() {
		return errors.New("data file is too long")
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

type data struct {
	*os.File

	buf [8]byte
}

func (d *data) frameReader(ofst int64) (io.Reader, error) {
	var sz int64
	err := binary.Read(io.NewSectionReader(d, ofst, 8), binary.BigEndian, &sz)
	if err != nil {
		return nil, errors.Wrap(err, "error reading payload length")
	}

	return io.NewSectionReader(d, ofst+8, sz), nil
}

func (d *data) readFrame(data []byte, ofst int64) (int, error) {
	sr := io.NewSectionReader(d, ofst, 8)

	var sz int64
	err := binary.Read(sr, binary.BigEndian, &sz)
	if err != nil {
		return 0, errors.Wrap(err, "error reading payload length")
	}

	return d.ReadAt(data, ofst+8)
}

func (d *data) getFrameSize(ofst int64) (int64, error) {
	_, err := d.ReadAt(d.buf[:], ofst)
	if err != nil {
		return 0, errors.Wrap(err, "error reading payload length")
	}

	buf := bytes.NewBuffer(d.buf[:])

	var sz int64
	err = binary.Read(buf, binary.BigEndian, &sz)
	return sz, errors.Wrap(err, "error parsing payload length")
}

func (d *data) getFrame(ofst int64) ([]byte, error) {
	sz, err := d.getFrameSize(ofst)
	if err != nil {
		return nil, errors.Wrap(err, "error getting frame size")
	}

	data := make([]byte, sz)
	_, err = d.readFrame(data, ofst)
	return data, errors.Wrap(err, "error reading frame")
}

func (d *data) append(data []byte) (int64, error) {
	ofst, err := d.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, errors.Wrap(err, "failed to seek to end of file")
	}

	err = binary.Write(d, binary.BigEndian, int64(len(data)))
	if err != nil {
		return 0, errors.Wrap(err, "writing length prefix failed")
	}

	_, err = d.Write(data)
	return ofst, errors.Wrap(err, "error writing data")
}

type offset struct {
	*os.File
}

func (o *offset) readOffset(seq margaret.Seq) (int64, error) {
	_, err := o.Seek(seq.Seq()*8, io.SeekStart)
	if err != nil {
		return 0, errors.Wrap(err, "seek failed")
	}

	var ofst int64
	err = binary.Read(o, binary.BigEndian, &ofst)
	return ofst, errors.Wrap(err, "error reading offset")
}

func (o *offset) readLastOffset() (int64, margaret.Seq, error) {
	stat, err := o.Stat()
	if err != nil {
		return 0, margaret.SeqEmpty, errors.Wrap(err, "stat failed")
	}

	sz := stat.Size()
	if sz == 0 {
		return 0, margaret.SeqEmpty, nil
	}

	// this should be off-by-one-error-free:
	// sz is 8 when there is one entry, and the first entry has seq 0
	seqOfst := margaret.BaseSeq(sz/8 - 1)

	var ofstData int64
	err = binary.Read(io.NewSectionReader(o, sz-8, 8), binary.BigEndian, &ofstData)
	if err != nil {
		return 0, margaret.SeqEmpty, errors.Wrap(err, "error reading entry")
	}

	return ofstData, seqOfst, nil
}

func (o *offset) append(ofst int64) (margaret.Seq, error) {
	ofstOfst, err := o.Seek(0, io.SeekEnd)
	seq := margaret.BaseSeq(ofstOfst / 8)
	if err != nil {
		return seq, errors.Wrap(err, "could not seek to end of file")
	}

	err = binary.Write(o, binary.BigEndian, ofst)
	return seq, errors.Wrap(err, "error writing offset")
}

type journal struct {
	*os.File
}

func (j *journal) readSeq() (margaret.Seq, error) {
	stat, err := j.Stat()
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "stat failed")
	}

	switch sz := stat.Size(); sz {
	case 0:
		return margaret.SeqEmpty, nil
	case 8:
		// continue after switch
	default:
		return margaret.SeqEmpty, errors.Errorf("expected file size of 8B, got %dB", sz)
	}

	_, err = j.Seek(0, io.SeekStart)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "could not seek to start of file")
	}

	var seq margaret.BaseSeq
	err = binary.Read(j, binary.BigEndian, &seq)
	return seq, errors.Wrap(err, "error reading seq")
}

func (j *journal) bump() (margaret.Seq, error) {
	seq, err := j.readSeq()
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "error reading old journal value")
	}

	_, err = j.Seek(0, io.SeekStart)
	if err != nil {
		return margaret.SeqEmpty, errors.Wrap(err, "could not seek to start of file")
	}

	seq = margaret.BaseSeq(seq.Seq() + 1)
	err = binary.Write(j, binary.BigEndian, seq)
	return seq, errors.Wrap(err, "error writing seq")
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
