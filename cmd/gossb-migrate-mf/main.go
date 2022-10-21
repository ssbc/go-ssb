// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

// gossb-migrate-mf is a metafeed migration utility.
// Given a go-sbot running only classic (or gabby) feeds, the migration utility, after running, will upgrade the
// instance to be metafeed-enabled.
// A root metafeed will be created, and the main feed will be searched for about and contact messages. Index feeds for
// those message types will be registered and populated.
// A metafeed-upgrading message `metafeed/announce` will be posted to the main feed (and be cross-signed by the root
// metafeed's secret), and the metafeed will register the main feed as one of its subfeeds.
package main

import (
	"flag"
	"log"

	"github.com/ssbc/go-ssb/cmd/gossb-migrate-mf/migrate"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	var err error
	var ssbdir string
	defaultDir, err := migrate.GetDefaultPath()
	check(err)
	flag.StringVar(&ssbdir, "ssbdir", defaultDir, "the location for your ssb secret & go-ssb installation")
	flag.Parse()

	err = migrate.Run(ssbdir)
	check(err)
}
