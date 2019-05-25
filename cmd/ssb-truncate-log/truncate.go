package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"go.cryptoscope.co/margaret"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/offset2"
	"go.cryptoscope.co/ssb/message/legacy"
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
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: migrate2 <from> <to> <limit>")
		os.Exit(1)
	}
	fromPath := os.Args[1]
	toPath := os.Args[2]

	limit, err := strconv.Atoi(os.Args[3])
	check(err)

	from, err := offset2.Open(fromPath, msgpack.New(&legacy.OldStoredMessage{}))
	check(err)

	to, err := offset2.Open(toPath, msgpack.New(&legacy.OldStoredMessage{}))
	check(err)

	seq, err := from.Seq().Value()
	check(err)

	fmt.Println("element count in source log:", seq)
	start := time.Now()
	src, err := from.Query(margaret.Limit(limit))
	check(err)
	err = src.(luigi.PushSource).Push(context.TODO(), luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err == (luigi.EOS{}) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "push failed")
		}

		seq, err := to.Append(v)
		fmt.Print("\r", seq)
		return err
	}))
	check(err)
	fmt.Println()
	fmt.Println("copy done after:", time.Since(start))

	toSeq, err := to.Seq().Value()
	check(err)

	fmt.Println("target has", toSeq)
}
