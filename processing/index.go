package processing

import (
	"context"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

type Index struct {
	procs []MessageProcessor
}

func (idx Index) Pour(ctx context.Context, v interface{}) error {
	var (
		sw  = v.(margaret.SeqWrapper)
		msg = sw.Value().(ssb.Message)
		seq = sw.Seq()

		err error
	)

	for _, proc := range idx.procs {
		err = proc.ProcessMessage(ctx, msg, seq)
		if err != nil {
			return err
		}
	}

	return nil
}

func (idx Index) Close() error {
	for _, proc := range idx.procs {
		err := proc.Close(context.TODO())
		if err != nil {
			return err
		}
	}

	return nil
}
