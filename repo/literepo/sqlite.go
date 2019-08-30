//Package literepo implements a log with userfeeds ontop of sqlite using a fitting schema to give easy access to userFeeds multilog as well
package literepo

import (
	"database/sql"
	"os"
	"path/filepath"

	"go.cryptoscope.co/ssb/message/multimsg"

	"go.cryptoscope.co/ssb"

	"github.com/Masterminds/squirrel"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/margaret"
)

func Open(path string) (*sqliteLog, error) {
	s, err := os.Stat(path)
	if os.IsNotExist(err) {
		if filepath.Dir(path) == "" {
			path = "."
		}
		err = os.MkdirAll(path, 0700)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create path location")
		}
		s, err = os.Stat(path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to stat created path location")
		}
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to stat path location")
	}
	if s.IsDir() {
		path = filepath.Join(path, "log.db")
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open sqlite file: %s", path)
	}
	var version int
	err = db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if err == sql.ErrNoRows || version == 0 { // new file or old schema

		if _, err := db.Exec(schemaVersion1); err != nil {
			return nil, errors.Wrap(err, "margaret/sqlite: failed to init schema v1")
		}

	} else if err != nil {
		return nil, errors.Wrapf(err, "margaret/sqlite: schema version lookup failed %s", path)
	}

	return &sqliteLog{
		db: db,
	}, err
}

const schemaVersion1 = `
CREATE TABLE authors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    author TEXT UNIQUE
);

CREATE TABLE messagekeys (
    id INTEGER PRIMARY KEY,
    key TEXT UNIQUE
);

CREATE TABLE messages (
    msg_id               INTEGER PRIMARY KEY,
    author_id            INTEGER NOT NULL,
    sequence             integer,
    type                 text NOT NULL,             -- needed so that we know what tables to merge for more info
    received_at          real NOT NULL,
	claimed_at           real DEFAULT 0,
	raw 				 blob,

	-- this is a very naive way of requiring _unforked chain_
    -- a proper implementation would require checks on the previous field
    CONSTRAINT simple_chain UNIQUE ( author_id, sequence ),

    FOREIGN KEY ( author_id ) REFERENCES authors( "id" ),
    FOREIGN KEY ( msg_id ) REFERENCES msgkeys( "id" )
);

CREATE INDEX author ON messages ( author_id );
CREATE INDEX received ON messages ( received_at );

PRAGMA user_version = 1;
`

type sqliteLog struct {
	db *sql.DB
}

func (sl sqliteLog) DB() *sql.DB { return sl.db }

func (sl sqliteLog) Close() error {
	return sl.db.Close()
}

func (sl sqliteLog) Seq() luigi.Observable {
	var cnt uint
	err := sl.db.QueryRow(`SELECT count(*) from messages;`).Scan(&cnt)
	if err != nil {
		return luigi.NewObservable(err)
	}
	return luigi.NewObservable(margaret.BaseSeq(cnt - 1))
}

func (sl sqliteLog) Get(s margaret.Seq) (interface{}, error) {
	var data []byte
	err := sl.db.QueryRow(`SELECT raw from messages where msg_id = ?`, s.Seq()+1).Scan(&data)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return margaret.SeqEmpty, nil
		}
		return nil, errors.Wrapf(err, "sqlite/get(%d): failed to execute query", s.Seq())
	}
	var mm multimsg.MultiMessage
	err = mm.UnmarshalBinary(data)
	return &mm, errors.Wrapf(err, "sqlite/get(%d): failed to decode value", s.Seq())
}

func (sl sqliteLog) Append(val interface{}) (margaret.Seq, error) {
	msg, ok := val.(multimsg.MultiMessage)
	if !ok {
		return nil, errors.Errorf("sql only supports adding messages (got %T)", val)
	}

	msgID, err := idForMessage(sl.db, msg.Key(), true)
	if err != nil {
		return nil, errors.Wrapf(err, "sqlite/append: failed to get ID for msg %q", msg.Key().Ref())
	}

	authorID, err := idForAuthor(sl.db, msg.Author(), true)
	if err != nil {
		return nil, errors.Wrapf(err, "sqlite/append: failed to get ID for author %q", msg.Author().Ref())
	}

	rawData, err := msg.MarshalBinary()
	if err != nil {
		return nil, errors.Wrap(err, "sqlite/append: failed to make raw data from multimsg")
	}

	res, err := sl.db.Exec(`insert into messages (msg_id, author_id, sequence, type, received_at, claimed_at, raw) VALUES(?,?,?,?,?,?,?)`,
		msgID, authorID, msg.Seq(), "unset",
		msg.Received().Unix(), msg.Claimed().Unix(),
		rawData)
	if err != nil {
		return nil, errors.Wrap(err, "sqlite/append: failed insert new value")
	}

	newID, err := res.LastInsertId()
	if err != nil {
		return nil, errors.Wrap(err, "sqlite/append: failed to establish ID")
	}
	return margaret.BaseSeq(newID - 1), nil
}

func (sl sqliteLog) Query(specs ...margaret.QuerySpec) (luigi.Source, error) {
	qry := &sqliteQry{
		builder: squirrel.Select("raw").From("messages"),
		db:      sl.db,
		rows:    nil,
	}

	for _, s := range specs {
		err := s(qry)
		if err != nil {
			return nil, err
		}
	}

	return qry, nil
}

func (sl sqliteLog) Null(s margaret.Seq) error {
	rows, err := sl.db.Exec(`UPDATE messages SET raw = null where id = ?`, s.Seq()+1)
	if err != nil {
		return errors.Wrap(err, "sqlite/null: failed to execute update query")
	}
	affected, err := rows.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "sqlite/null: rows affected failed")
	}
	if affected != 1 {
		return errors.Errorf("sqlite/null: not one row affected but %d", affected)
	}
	return nil
}

func idForMessage(db *sql.DB, ref *ssb.MessageRef, make bool) (int64, error) {
	row := squirrel.Select("id").From("messagekeys").Where("key = ?", ref.Ref()).Limit(1).RunWith(db).QueryRow()

	var msgID int64
	err := row.Scan(&msgID)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows && make {
			res, err := db.Exec(`INSERT INTO messagekeys (key) VALUES(?)`, ref.Ref())
			if err != nil {
				return -1, err
			}

			newID, err := res.LastInsertId()
			if err != nil {
				return -1, err
			}
			return newID, nil
		}
		return -1, err
	}
	return msgID, nil
}

func idForAuthor(db *sql.DB, ref *ssb.FeedRef, make bool) (int64, error) {
	row := squirrel.Select("id").From("authors").Where("author = ?", ref.Ref()).Limit(1).RunWith(db).QueryRow()

	var msgID int64
	err := row.Scan(&msgID)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows && make {
			res, err := db.Exec(`INSERT INTO authors (author) VALUES(?)`, ref.Ref())
			if err != nil {
				return -1, err
			}

			newID, err := res.LastInsertId()
			if err != nil {
				return -1, err
			}
			return newID, nil
		}

		return -1, err
	}
	return msgID, nil
}

/*
private func msgID(from: MessageIdentifier, make: Bool = false) throws -> Int64 {

	var msgID: Int64
	if let msgKeysRow = try db.pluck(self.msgKeys.filter(colKey == from)) {
		msgID = msgKeysRow[colID]
	} else {
		if make {
			msgID = try db.run(self.msgKeys.insert(
				colKey <- from
			))
		} else {
			throw ViewDatabaseError.unknownMessage(from)
		}
	}
	return msgID
}

private func msgKey(id: Int64) throws -> MessageIdentifier {
	guard let db = self.openDB else {
		throw ViewDatabaseError.notOpen
	}

	var msgKey: MessageIdentifier
	if let msgKeysRow = try db.pluck(self.msgKeys.filter(colID == id)) {
		msgKey = msgKeysRow[colKey]
	} else {
		throw ViewDatabaseError.unknownReferenceID(id)
	}
	return msgKey
}

private func authorID(from: Identity, make: Bool = false) throws -> Int64 {
	guard let db = self.openDB else {
		throw ViewDatabaseError.notOpen
	}
	var authorID: Int64
	if let authorRow = try db.pluck(self.authors.filter(colAuthor == from)) {
		authorID = authorRow[colID]
	} else {
		if make {
			authorID = try db.run(self.authors.insert(
				colAuthor <- from
			))
		} else {
			throw ViewDatabaseError.unknownAuthor(from)
		}
	}
	return authorID
}
*/
