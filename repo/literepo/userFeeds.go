package literepo

import (
	"context"
	"database/sql"
	"fmt"

	"go.cryptoscope.co/ssb"

	"github.com/Masterminds/squirrel"
	"github.com/pkg/errors"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
	"go.cryptoscope.co/ssb/message/multimsg"
	"go.cryptoscope.co/ssb/repo"
)

type userMulilog struct {
	db *sql.DB
}

func (sl sqliteLog) UserFeeds() repo.MakeMultiLog {
	return func(_ repo.Interface) (multilog.MultiLog, repo.ServeFunc, error) {
		return &userMulilog{
			db: sl.db,
		}, nil, nil
	}
}

func (uml userMulilog) Get(addr librarian.Addr) (margaret.Log, error) {
	ref, err := ssb.ParseFeedRef(string(addr))
	if err != nil {
		return nil, err
	}
	return &userSublog{
		db:   uml.db,
		user: ref,
	}, nil
}

func (uml userMulilog) List() ([]librarian.Addr, error) {
	return nil, errors.Errorf("TODO:list")
}

func (uml userMulilog) Close() error {
	return nil
}

type userSublog struct {
	db *sql.DB

	user *ssb.FeedRef
}

func (log *userSublog) Seq() luigi.Observable {
	id, err := idForAuthor(log.db, log.user, true)
	if err != nil {
		panic(err)
	}

	var cnt int
	err = log.db.QueryRow(`SELECT sequence from messages where author_id = ? order by sequence desc limit 1`, id).Scan(&cnt)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return luigi.NewObservable(margaret.SeqEmpty)
		}
		panic(errors.Wrap(err, "seq: failed to establish msg count"))
	}

	fmt.Println(errors.Errorf("TODO:seq:%s:%d", log.user.Ref(), id))

	return luigi.NewObservable(margaret.BaseSeq(cnt))
}

func (log *userSublog) Get(seq margaret.Seq) (interface{}, error) {

	id, err := idForAuthor(log.db, log.user, true)
	if err != nil {
		return nil, err
	}

	var data []byte
	err = log.db.QueryRow(`SELECT raw from messages where author_id = ? and sequence = ?`, id, seq.Seq()).Scan(&data)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, luigi.EOS{}
		}
		return nil, errors.Wrapf(err, "failed get specific msg: %d", seq.Seq())
	}

	var mm multimsg.MultiMessage
	err = mm.UnmarshalBinary(data)

	fmt.Println(errors.Errorf("TODO:get:%s:%d", log.user.Ref(), id))
	return &mm, err
}

func (log *userSublog) Query(specs ...margaret.QuerySpec) (luigi.Source, error) {
	qry := &userQuery{
		db: log.db,

		// lt:      margaret.SeqEmpty,
		// nextSeq: margaret.SeqEmpty,

		// limit: -1, //i.e. no limit
	}

	for _, spec := range specs {
		err := spec(qry)
		if err != nil {
			return nil, err
		}
	}

	return qry, nil
}

func (log *userSublog) Append(v interface{}) (margaret.Seq, error) {
	return nil, errors.Errorf("userSublog: read-only! should be filled by inserts in the main log")
}

type userQuery struct {
	user librarian.Addr

	db   *sql.DB
	rows *sql.Rows

	builder squirrel.SelectBuilder

	seqWrap bool
}

func (sq *userQuery) Next(ctx context.Context) (interface{}, error) {
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

func (sq *userQuery) Gt(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.Gt{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *userQuery) Gte(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.GtOrEq{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *userQuery) Lt(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.Lt{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *userQuery) Lte(s margaret.Seq) error {
	sq.builder = sq.builder.Where(squirrel.LtOrEq{"msg_id": s.Seq() + 1})
	return nil
}

func (sq *userQuery) Limit(n int) error {
	sq.builder = sq.builder.Limit(uint64(n))
	return nil
}

func (sq *userQuery) Live(live bool) error {
	if live {
		return margaret.ErrUnsupported("sqlite queries don't support live results")
	}
	return nil
}

func (sq *userQuery) SeqWrap(wrap bool) error {
	if wrap && sq.seqWrap == false { // only wrap once
		sq.builder = sq.builder.Columns("msg_id")
		sq.seqWrap = wrap
	}
	return nil
}

func (sq *userQuery) Reverse(yes bool) error {
	if yes {
		sq.builder = sq.builder.OrderBy("msg_id desc")
	}
	return nil
}
