package badger

import (
	"context"
	"encoding/binary"
	"sync"

	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type query struct {
	l   sync.Mutex
	log *sublog

	nextSeq, lt margaret.BaseSeq

	limit   int
	live    bool
	seqWrap bool
}

func (qry *query) Gt(s margaret.Seq) error {
	if qry.nextSeq > margaret.SeqEmpty {
		return errors.Errorf("lower bound already set")
	}

	qry.nextSeq = margaret.BaseSeq(s.Seq() + 1)
	return nil
}

func (qry *query) Gte(s margaret.Seq) error {
	if qry.nextSeq > margaret.SeqEmpty {
		return errors.Errorf("lower bound already set")
	}

	qry.nextSeq = margaret.BaseSeq(s.Seq())
	return nil
}

func (qry *query) Lt(s margaret.Seq) error {
	if qry.lt != margaret.SeqEmpty {
		return errors.Errorf("upper bound already set")
	}

	qry.lt = margaret.BaseSeq(s.Seq())
	return nil
}

func (qry *query) Lte(s margaret.Seq) error {
	if qry.lt != margaret.SeqEmpty {
		return errors.Errorf("upper bound already set")
	}

	qry.lt = margaret.BaseSeq(s.Seq() + 1)
	return nil
}

func (qry *query) Limit(n int) error {
	qry.limit = n
	return nil
}

func (qry *query) Live(live bool) error {
	qry.live = live
	return nil
}

func (qry *query) SeqWrap(wrap bool) error {
	qry.seqWrap = wrap
	return nil
}

func (qry *query) Next(ctx context.Context) (interface{}, error) {
	qry.l.Lock()
	defer qry.l.Unlock()

	if qry.limit == 0 {
		return nil, luigi.EOS{}
	}
	qry.limit--

	if qry.nextSeq == margaret.SeqEmpty {
		qry.nextSeq = 0
	}

	if qry.lt != margaret.SeqEmpty {
		if qry.nextSeq >= qry.lt {
			return nil, luigi.EOS{}
		}
	}

	// TODO: use iterator instead of getting sequentially

	nextSeqBs := make([]byte, 8)
	binary.BigEndian.PutUint64(nextSeqBs, uint64(qry.nextSeq))
	prefix := make([]byte, len(qry.log.prefix))
	copy(prefix, qry.log.prefix)
	key := append(prefix, nextSeqBs...)

	var v interface{}

	err := qry.log.mlog.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return errors.Wrap(err, "error getting item")
		}

		data, err := item.Value()
		if err != nil {
			return errors.Wrap(err, "error getting value")
		}

		v, err = qry.log.mlog.codec.Unmarshal(data)
		return errors.Wrap(err, "error unmarshaling data")
	})
	if err != nil {
		// if key is not found, we haven't written that far yet
		if errors.Cause(err) == badger.ErrKeyNotFound {
			// abort if not a live query, else wait until it's written
			if !qry.live {
				return nil, luigi.EOS{}
			}

			wait := make(chan struct{})
			closed := make(chan struct{})

			var cancel func()
			cancel = qry.log.seq.Register(luigi.FuncSink(
				func(ctx context.Context, v interface{}, err error) error {
					if err != nil {
						close(closed)
						return nil
					}
					if v.(margaret.Seq).Seq() >= qry.nextSeq.Seq() {
						close(wait)
					}

					return nil
				}))
			defer cancel()

			err := func() error {
				qry.l.Unlock()
				defer qry.l.Lock()

				select {
				case <-wait:
				case <-closed:
					return errors.New("seq observable closed")
				case <-ctx.Done():
					return errors.Wrap(ctx.Err(), "cancelled while waiting for value to be written")
				}
				return nil
			}()
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.Wrap(err, "error in read transaction")
		}
	}

	defer func() { qry.nextSeq++ }()

	if qry.seqWrap {
		return margaret.WrapWithSeq(v, qry.nextSeq), nil
	}

	return v, nil
}
