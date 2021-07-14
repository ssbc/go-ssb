// SPDX-License-Identifier: MIT

// ssb-drop-feed nulls entries of one particular feed from repo
// there is no warning or undo
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func check(err error, msg string, args ...interface{}) {
	if err != nil {
		fmt.Printf(msg+"\n", args...)
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
		fmt.Fprintf(os.Stderr, "usage: %s <repo> <@feed=>\n", os.Args[0])
		os.Exit(1)
	}

	r := repo.New(os.Args[1])

	var inputRefs []refs.FeedRef
	if os.Args[2] == "-" {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			line := s.Text()
			fr, err := refs.ParseFeedRef(line)
			check(err, "failed to parse %q argument", line)
			inputRefs = append(inputRefs, fr)
		}
		check(s.Err(), "stdin scanner failed")
	} else {

		fr, err := refs.ParseFeedRef(os.Args[2])
		check(err, "failed to parse feed argument")
		inputRefs = append(inputRefs, fr)
	}

	rmbot, err := sbot.New(
		sbot.WithRepoPath(os.Args[1]),
		sbot.WithUNIXSocket())
	check(err, "failed to open bot")

	for i, fr := range inputRefs {
		start := time.Now()

		err := rmbot.NullFeed(fr)
		check(err, "failed to null feed: %s", fr.Ref())
		log.Printf("feed(%d) %s nulled (took %v)", i, fr.Ref(), time.Since(start))
	}

	rmbot.Shutdown()
	err = rmbot.Close()
	check(err, "failed to close the bot")

	start := time.Now()
	err = sbot.DropIndicies(r)
	check(err, "failed to drop indexes")
	log.Println("idexes dropped", time.Since(start))

	start = time.Now()
	err = sbot.RebuildIndicies(os.Args[1])
	check(err, "failed to rebuild indexes")
	log.Println("idexes rebuilt", time.Since(start))
}
