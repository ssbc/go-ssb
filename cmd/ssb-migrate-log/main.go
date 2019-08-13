package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret/codec/msgpack"
	"go.cryptoscope.co/margaret/offset2"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/message/legacy"
	"go.cryptoscope.co/ssb/message/multimsg"
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
		fmt.Fprintln(os.Stderr, "usage: migrate2 <from> <to>")
		os.Exit(1)
	}
	fromPath := os.Args[1]
	toPath := os.Args[2]

	from, err := offset2.Open(fromPath, msgpack.New(&legacy.OldStoredMessage{}))
	check(err)

	to, err := offset2.Open(toPath, msgpack.New(&multimsg.MultiMessage{}))
	to = multimsg.NewWrappedLog(to)

	seq, err := from.Seq().Value()
	check(err)

	fmt.Println("element count in source log:", seq)
	start := time.Now()
	src, err := from.Query()
	check(err)

	var got []string

	err = src.(luigi.PushSource).Push(context.TODO(), luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err == (luigi.EOS{}) {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "push failed")
		}

		msg := v.(legacy.OldStoredMessage)

		got = append(got, msg.Key.Ref())

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

	newTarget, err := to.Query()
	check(err)

	i := 0
	for {
		v, err := newTarget.Next(context.TODO())
		if luigi.IsEOS(err) {
			break
		} else if err != nil {
			check(err)
		}

		msg := v.(ssb.Message)

		if got[i] != msg.Key().Ref() {
			check(fmt.Errorf("migrate failed - msg%d diverges", i))
		}
		fmt.Print("\r", i)
		i++
	}

	fmt.Println("\ncompare done")
}
