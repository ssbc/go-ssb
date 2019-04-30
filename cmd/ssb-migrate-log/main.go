// this tool copies an offset log v1 (from) into a v2 (to)
// v1 was a single file log
// v2 offers a much denser encoding (less gaps)
package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/framing/lengthprefixed"
	"go.cryptoscope.co/margaret/offset"
	"go.cryptoscope.co/margaret/offset2"
	"go.cryptoscope.co/ssb/message"
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
		fmt.Fprintln(os.Stderr, "usage: migrate <from> <to>")
		os.Exit(1)
	}
	fromPath := os.Args[1]
	toPath := os.Args[2]

	fromFile, err := os.OpenFile(fromPath, os.O_RDWR, 0600)
	check(err)

	from, err := offset.New(
		fromFile, lengthprefixed.New32(16*1024), msgpack.New(&message.StoredMessage{}))
	check(err)

	to, err := offset2.Open(toPath, msgpack.New(&message.StoredMessage{}))
	check(err)

	seq, err := from.Seq().Value()
	check(err)

	fmt.Println("element count in source log:", seq)

	src, err := from.Query()
	check(err)
	err = src.(luigi.PushSource).Push(context.TODO(), luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err == (luigi.EOS{}) {
			return nil
		}
		if err != nil {
			return err
		}

		seq, err := to.Append(v)
		fmt.Print("\r", seq)
		return err
	}))
	check(err)
	fmt.Println()
}
