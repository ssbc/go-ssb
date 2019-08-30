package literepo

import (
	"context"
	"database/sql"

	"go.cryptoscope.co/ssb/message/multimsg"

	"github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/margaret"
)

type sqliteQry struct {
	db   *sql.DB
	rows *sql.Rows

	builder squirrel.SelectBuilder

	seqWrap bool
}

func (sq *sqliteQry) Next(ctx context.Context) (interface{}, error) {
	if sq.rows == nil {
		var err error
		sq.rows, err = sq.builder.RunWith(sq.db).Query()
		if err != nil {
			return nil, errors.Wrap(err, "sqlite/query: failed to init rows")
		}
	}

	if !sq.rows.Next() {
		return nil, luigi.EOS{}
	}

	var (
		id   int64
		data []byte
		err  error
	)
	if sq.seqWrap {
		err = sq.rows.Scan(&data, &id)
	} else {
		err = sq.rows.Scan(&data)
	}
	if err != nil {
		return nil, errors.Wrap(err, "sqlite/query: failed to scan data")
	}
	if len(data) == 0 {
		return nil, margaret.ErrNulled
	}

	var mm multimsg.MultiMessage
	err = mm.UnmarshalBinary(data)
	if err != nil {
		return nil, errors.Wrap(err, "sqlite/query: next failed to decode multimsg")
	}

	var v interface{} = mm
	if sq.seqWrap {
		v = margaret.WrapWithSeq(v, margaret.BaseSeq(id-1))
	}
	return v, nil
}

func (sq *sqliteQry) Gt(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.Gt{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *sqliteQry) Gte(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.GtOrEq{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *sqliteQry) Lt(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.Lt{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *sqliteQry) Lte(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.LtOrEq{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *sqliteQry) Limit(n int) error {
	sq.builder = sq.builder.Limit(uint64(n))
	return nil
}

func (sq *sqliteQry) Live(live bool) error {
	if live {
		return margaret.ErrUnsupported("sqlite queries don't support live results")
	}
	return nil
}

func (sq *sqliteQry) SeqWrap(wrap bool) error {
	if wrap && sq.seqWrap == false { // only wrap once
		sq.builder = sq.builder.Columns("msg_id")
		sq.seqWrap = wrap
	}
	return nil
}

func (sq *sqliteQry) Reverse(yes bool) error {
	if yes {
		sq.builder = sq.builder.OrderBy("msg_id desc")
	}
	return nil
}
