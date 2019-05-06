// ssb-drop-feed nulls entries of one particular feed from repo
// there is no warning or undo
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/pkg/errors"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
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
		fmt.Fprintf(os.Stderr, "usage: %s <repo> <@feed=...>\n", os.Args[0])
		os.Exit(1)
	}

	r := repo.New(os.Args[1])
	fr, err := ssb.ParseFeedRef(os.Args[2])
	check(errors.Wrap(err, "failed to parse feed argument"))

	err = sbot.NullFeed(r, fr)
	check(err)
}
