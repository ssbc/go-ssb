// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/multilog"
	multifs "go.cryptoscope.co/margaret/multilog/roaring/fs"
	"go.mindeco.de/logging"
)

var check = logging.CheckFatal

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <dir> (hasAddr)", os.Args[0])
		os.Exit(1)
	}
	logging.SetupLogging(nil)
	log := logging.Logger(os.Args[0])

	dir := os.Args[1]

	mlog, err := multifs.NewMultiLog(dir)
	check(err)

	addrs, err := mlog.List()
	if err != nil {
		check(fmt.Errorf("error listing multilog: %w", err))
	}
	log.Log("mlog", "opened", "list#", len(addrs))
	for i, a := range addrs {

		sublog, err := mlog.Get(a)
		check(err)
		seqv, err := sublog.Seq().Value()
		check(err)
		log.Log("i", i, "addr", string(a), "seq", seqv)
	}

	// check has
	if len(os.Args) > 2 {
		addr := librarian.Addr(os.Args[2])

		has, err := multilog.Has(mlog, addr)
		log.Log("mlog", "has", "addr", string(addr), "has?", has, "hasErr", err)

		bmap, err := mlog.LoadInternalBitmap(addr)
		check(err)
		fmt.Println(bmap.GetCardinality())
		fmt.Println(bmap.String())
	}

	check(mlog.Close())
}
