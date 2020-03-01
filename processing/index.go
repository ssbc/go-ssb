package processing

import (
	"context"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/ssb"
)

// Index is a luigi Sink that consumes margaret.SeqWrapper values that contain ssb.Messages, and
// passes these into each MessageProcessor in Procs.
type Index struct {
	Procs []MessageProcessor
}

// Pour feeds a seqwrapped message into all message processors.
func (idx Index) Pour(ctx context.Context, v interface{}) error {
	var (
		sw  = v.(margaret.SeqWrapper)
		msg = sw.Value().(ssb.Message)
		seq = sw.Seq()

		err error
	)

	for _, proc := range idx.Procs {
		err = proc.ProcessMessage(ctx, msg, seq)
		if err != nil {
			return err
		}
	}

	return nil
}

// Close closes all attached MessageProcessors.
func (idx Index) Close() error {
	for _, proc := range idx.Procs {
		err := proc.Close(context.TODO())
		if err != nil {
			return err
		}
	}

	return nil
}
