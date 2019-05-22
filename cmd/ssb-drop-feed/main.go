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
		fmt.Fprintf(os.Stderr, "usage: %s <repo> <@feed=>\n", os.Args[0])
		os.Exit(1)
	}

	r := repo.New(os.Args[1])

	var refs []*ssb.FeedRef
	if os.Args[2] == "-" {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			line := s.Text()
			fr, err := ssb.ParseFeedRef(line)
			check(errors.Wrapf(err, "failed to parse %q argument", line))
			refs = append(refs, fr)
		}
		check(errors.Wrap(s.Err(), "stdin scanner failed"))
	} else {

		fr, err := ssb.ParseFeedRef(os.Args[2])
		check(errors.Wrap(err, "failed to parse feed argument"))
		refs = append(refs, fr)
	}

	for i, fr := range refs {
		start := time.Now()
		err := sbot.NullFeed(r, fr)
		check(err)
		log.Printf("feed(%d) %s nulled (took %v)", i, fr.Ref(), time.Since(start))
	}

	start := time.Now()
	err := sbot.DropIndicies(r)
	check(err)
	log.Println("idexes dropped", time.Since(start))

	start = time.Now()
	err = sbot.RebuildIndicies(os.Args[1])
	check(err)
	log.Println("idexes rebuilt", time.Since(start))
}
