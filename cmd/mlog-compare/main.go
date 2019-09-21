package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"

	"github.com/pkg/errors"
	"github.com/shurcooL/go-goon"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"

	"go.cryptoscope.co/ssb/multilogs"
	"go.cryptoscope.co/ssb/repo"
)

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	fmt.Fprintln(os.Stderr, "occurred at")
	debug.PrintStack()
	os.Exit(1)
}

var ctx = context.Background()

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mlog-cmp <repo>")
		os.Exit(1)
	}
	repoPath := os.Args[1]

	r := repo.New(repoPath)

	rootLog, err := repo.OpenLog(r)
	check(err)

	badger, serve, err := repo.OpenBadgerMultiLog(r, "users_badger", multilogs.UserFeedsUpdate)
	check(err)

	err = serve(ctx, rootLog, false)
	badgerMap := mapListOfAllSequencesByAddr(badger)
	check(badger.Close())
	fmt.Println("badger ready")

	roar, serve, err := multilogs.OpenUserFeeds(r)
	check(err)
	err = serve(ctx, rootLog, false)
	check(err)
	roaringMap := mapListOfAllSequencesByAddr(roar)
	fmt.Println("roaring ready")

	compare := func(b sequenceMap, name string) {

		for addr, seqsBadger := range badgerMap {
			fmt.Printf("checking %x\n", addr)

			seqsRoaring, ok := b[addr]
			if !ok {
				check(errors.Errorf("key not found in roaring map: %x\n", addr))
			}

			nr := len(seqsRoaring)
			nb := len(seqsBadger)
			if diff := nr - nb; diff != 0 {
				fmt.Printf("difference in length on %x (diff:%d) (badger:%d)\n", addr, diff, nb)
			}
			var wrong bool
			for i, sb := range seqsBadger {
				if i > nr {
					fmt.Println("breaking, roaring too short...")
					break
				}

				sr := seqsRoaring[i]

				srv := sr.Seq()
				sbv := sb.Seq()
				if diff := srv - sbv; diff != 0 {
					fmt.Printf("compare(%03d) %d %d (diff: %d)\n", i, srv, sbv, diff)
					wrong = true
				}
			}
			if wrong {
				fname := fmt.Sprintf("%x.wrong.%s", addr, name)
				ioutil.WriteFile(
					fname,
					[]byte(goon.Sdump(seqsBadger)),
					0700)
				fmt.Println("wrote", fname)
			}
		}
	}
	// compare(roaringMapCompressed, "comp")
	compare(roaringMap, "uncomp")

	fmt.Println("compare done")

}

type sequenceMap map[librarian.Addr][]margaret.Seq

func mapListOfAllSequencesByAddr(ml multilog.MultiLog) sequenceMap {
	seqMap := make(sequenceMap)
	bAddrs, err := ml.List()
	check(err)

	for _, addr := range bAddrs {
		sublog, err := ml.Get(addr)
		check(err)

		src, err := sublog.Query()
		check(err)

		var svs []interface{}
		snk := luigi.NewSliceSink(&svs)

		err = luigi.Pump(ctx, snk, src)
		check(err)

		// sequence values behind interface
		seqs := make([]margaret.Seq, len(svs))
		for i, v := range svs {
			s, ok := v.(margaret.Seq)
			if !ok {
				check(errors.Errorf("wrong type from multilog: %T", v))
			}
			seqs[i] = s
		}

		seqMap[addr] = seqs
	}
	return seqMap
}
