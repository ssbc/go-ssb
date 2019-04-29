/* Package statematrix stores and provides useful operations on an state matrix for a epidemic broadcast tree protocol.

The state matrix represents multiple _network frontiers_ (or vector clock).

This version uses a SQL because that seems much handier to handle such an irregular sparse matrix.

Q:
* do we need a 2nd _told us about_ table?

*/
package statematrix

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/internal/numberedfeeds"
	refs "go.mindeco.de/ssb-refs"
	"modernc.org/ql"
)

func init() {
	ql.RegisterDriver2()
}

type StateMatrix struct {
	store *sql.DB

	feedidx *numberedfeeds.Index
}

func New(path string, idx *numberedfeeds.Index) (*StateMatrix, error) {
	db, err := sql.Open("ql2", path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open database")
	}

	var version uint
	err = db.QueryRow(`SELECT version from ebtMeta where space = ?`, "ssb").Scan(&version)
	if err == sql.ErrNoRows || version == 0 { // new file or old schema
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}

		if _, err := tx.Exec(schemaVersion1); err != nil {
			return nil, errors.Wrap(err, "persist/ql: failed to init schema v1")
		}
		err = tx.Commit()
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, errors.Wrapf(err, "persist/ql: schema version lookup failed %s", path)
	}

	return &StateMatrix{
		store:   db,
		feedidx: idx,
	}, nil
}

type HasLongerResult struct {
	Peer *refs.FeedRef
	Feed *refs.FeedRef
	Len  uint64
}

func (hlr HasLongerResult) String() string {
	return fmt.Sprintf("%s: %s:%d", hlr.Peer.ShortRef(), hlr.Feed.ShortRef(), hlr.Len)
}

func (sm StateMatrix) HasLonger() ([]HasLongerResult, error) {
	rows, err := sm.store.Query(`
SELECT all.peer, all.feed, all.flen 
  FROM ebtState as all,
  (SELECT feed, flen FROM ebtState where peer = 0) as my
  WHERE all.feed == my.feed AND all.flen > my.flen
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []HasLongerResult

	for rows.Next() {
		var (
			hlr      HasLongerResult
			pid, fid uint64
		)
		err := rows.Scan(&pid, &fid, &hlr.Len)
		if err != nil {
			return nil, err
		}

		hlr.Feed, err = sm.feedidx.FeedFor(fid)
		if err != nil {
			return nil, err
		}

		hlr.Peer, err = sm.feedidx.FeedFor(pid)
		if err != nil {
			return nil, err
		}

		res = append(res, hlr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// all the feeds a peer wants to recevie messages for
func (sm StateMatrix) WantsList(peer *refs.FeedRef) ([]*refs.FeedRef, error) {
	nPeer, err := sm.feedidx.NumberFor(peer)
	if err != nil {
		return nil, err
	}

	// set of shared feeds
	rows, err := sm.store.Query(`select feed where peer == uint64(?1) and rx == true`, nPeer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*refs.FeedRef

	for rows.Next() {
		var (
			fid uint64
		)
		err := rows.Scan(&fid)
		if err != nil {
			return nil, err
		}

		fref, err := sm.feedidx.FeedFor(fid)
		if err != nil {
			return nil, err
		}

		res = append(res, fref)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

func (sm StateMatrix) WantsFeed(peer, feed *refs.FeedRef, weHave uint64) (bool, error) {
	nPeer, err := sm.feedidx.NumberFor(peer)
	if err != nil {
		return false, err
	}

	nFeed, err := sm.feedidx.NumberFor(feed)
	if err != nil {
		return false, err
	}

	var rx bool
	err = sm.store.QueryRow(`select rx from ebtState
		where peer == uint64(?1) and feed == uint64(?2) and flen < uint64(?3)`,
		nPeer, nFeed, weHave).Scan(&rx)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}

	return rx, nil
}

func (sm StateMatrix) Changed(self, peer *refs.FeedRef) (ssb.NetworkFrontier, error) {
	nSelf, err := sm.feedidx.NumberFor(self)
	if err != nil {
		return nil, err
	}

	nPeer, err := sm.feedidx.NumberFor(peer)
	if err != nil {
		return nil, err
	}

	// set of shared feeds
	rows, err := sm.store.Query(`
	select feed, flen from ebtState where peer == uint64(?1) and feed IN (
SELECT remote.feed
FROM ebtState as remote,
  (SELECT feed, flen FROM ebtState where peer == uint64(?1)) as my
WHERE remote.feed == my.feed AND remote.peer == uint64(?2)
)
`, nSelf, nPeer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res = make(ssb.NetworkFrontier)

	for rows.Next() {
		var (
			fid  uint64
			flen uint64
			rx   bool = true // TODO
		)
		err := rows.Scan(&fid, &flen)
		if err != nil {
			return nil, err
		}

		fref, err := sm.feedidx.FeedFor(fid)
		if err != nil {
			return nil, err
		}

		res[fref] = ssb.Note{
			Seq:       int64(flen),
			Replicate: flen != 0,
			Receive:   rx,
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

type ObservedFeed struct {
	Feed      *refs.FeedRef
	Len       uint64
	Receive   bool
	Replicate bool
}

func (sm StateMatrix) Fill(who *refs.FeedRef, feeds []ObservedFeed) error {
	tx, err := sm.store.Begin()
	if err != nil {
		return err
	}

	nWho, err := sm.feedidx.NumberFor(who)
	if err != nil {
		return err
	}

	for i, of := range feeds {
		nof, err := sm.feedidx.NumberFor(of.Feed)
		if err != nil {
			return err
		}
		if of.Replicate {
			// SAD UPSERT
			res, err := tx.Exec(`UPDATE ebtState 
				rx = bool(?4), flen = uint64(?3)
			where peer=uint64(?1) and feed=uint64(?2)`, nWho, nof, of.Len, of.Receive)
			if err != nil {
				return errors.Wrapf(err, "fill%d failed update ", i)
			}
			affn, err := res.RowsAffected()
			if err != nil {
				return errors.Wrapf(err, "fill%d failed affected", i)
			}
			if affn < 1 {
				_, err := tx.Exec(`INSERT INTO ebtState (peer, feed, flen, rx) VALUES (uint64(?1), uint64(?2), uint64(?3), ?4)`, nWho, nof, of.Len, of.Receive)
				if err != nil {
					return errors.Wrapf(err, "fill%d failed insert", i)
				}
			}
		} else {
			// seq == -1 means drop it
			_, err := tx.Exec(`DELETE FROM ebtState where peer=uint64(?1) and feed=uint64(?2)`, nWho, nof)
			if err != nil {
				return errors.Wrapf(err, "fill%d drop failed", i)
			}
		}
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "fill failed commit")
	}
	return nil
}

const schemaVersion1 = `
BEGIN TRANSACTION;

CREATE TABLE ebtMeta (
  space string,
  version uint32,
);
CREATE UNIQUE INDEX ebtVersioning ON ebtMeta (space);

CREATE TABLE ebtState (
	peer uint64 NOT NULL,
	feed uint64 NOT NULL,
	
	flen uint64 NOT NULL,

	rx bool not null,
);

CREATE UNIQUE INDEX ebtPeers ON ebtState (peer, feed);
CREATE INDEX ebtLengths ON ebtState (feed,flen);

insert into ebtMeta VALUES ("ssb",1);
COMMIT;
`

func (sm StateMatrix) Close() error {
	return sm.store.Close()
}

func (sm StateMatrix) String() string {
	var sb strings.Builder

	//sm.store.QueryRow()

	fmt.Fprintf(&sb, "state matrix (%d:%d):", 1, 2)
	/*
		r, c := sm.mat.Dims()
			for ir := 0; ir < r; ir++ {
				for jc := 0; jc < c; jc++ {
					fmt.Fprintf(&sb, "%3.0f", sm.mat.At(ir, jc))
				}
				fmt.Fprintln(&sb)
			}
	*/
	return sb.String()
}
