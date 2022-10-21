// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

/*usefull to eradicate entries in offsetlog by hand.

Say you know 15, 28182 and 21881283 are probelematic.

call it like ./gossb-null-entry ~/.ssb-go 15 28182 21881283

- TODO: automate re-index rebuild of redueces (contacts) - you need to delete the folders by hand

- TODO: add feature to delete by author:seq

- TODO: add feature to delete by message hash (load the get index)
*/
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strconv"

	"github.com/ssbc/go-ssb/repo"
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

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: null-entry <logpath> <seq>...")
		os.Exit(1)
	}
	logPath := os.Args[1]

	var seqs = make([]int64, len(os.Args)-2)
	for i, seq := range os.Args[2:] {
		parsedSeq, err := strconv.Atoi(seq)
		check(err)

		seqs[i] = int64(parsedSeq)
	}

	repoFrom := repo.New(logPath)

	theLog, err := repo.OpenLog(repoFrom)
	check(err)

	fmt.Println("element count in source log:", theLog.Seq())

	fmt.Println("nulling:", seqs)

	for _, seq := range seqs {
		err = theLog.Null(seq)
		check(err)
	}
	check(theLog.Close())
	fmt.Println("all done")
}
