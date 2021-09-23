// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package luigiutils

import (
	"context"
	"fmt"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/muxrpc/v2"

	"go.cryptoscope.co/ssb/message/multimsg"
)

// NewGabbyStreamSink expects the values passing through to be of type multimsg.MultiMessage
// it then unpacks them as gabygrove, reencodes the transfer object to bytes
// and passes those as muxrpc codec.Body to the wrapped sink
func NewGabbyStreamSink(w muxrpc.ByteSinker) luigi.Sink {
	return luigi.FuncSink(func(_ context.Context, v interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}

		mm, err := getMultiMessage(v)
		if err != nil {
			return err
		}

		tr, ok := mm.AsGabby()
		if !ok {
			return fmt.Errorf("gabbyStream: wrong format type type")
		}

		trdata, err := tr.MarshalCBOR()
		if err != nil {
			return fmt.Errorf("gabbyStream: failed to marshal transfer object: %w", err)
		}

		w.SetEncoding(muxrpc.TypeBinary)
		_, err = w.Write(trdata)
		return err
	})
}

func NewBendyStreamSink(w muxrpc.ByteSinker) luigi.Sink {
	return luigi.FuncSink(func(_ context.Context, v interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return nil
			}
			return err
		}

		mm, err := getMultiMessage(v)
		if err != nil {
			return err
		}

		mf, ok := mm.AsMetaFeed()
		if !ok {
			return fmt.Errorf("gabbyStream: wrong format type type")
		}

		mfData, err := mf.MarshalBencode()
		if err != nil {
			return fmt.Errorf("gabbyStream: failed to marshal transfer object: %w", err)
		}

		w.SetEncoding(muxrpc.TypeBinary)
		_, err = w.Write(mfData)
		return err
	})
}

// NewSinkCounter returns a new Sink which increases the given counter when poured to.
func NewSinkCounter(counter *int, sink luigi.Sink) luigi.FuncSink {
	return func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			return err
		}

		err = sink.Pour(ctx, v)
		if err == nil {
			*counter++
		}
		return err
	}
}

func getMultiMessage(val interface{}) (*multimsg.MultiMessage, error) {
	var mm *multimsg.MultiMessage
	switch tv := val.(type) {
	case *multimsg.MultiMessage:
		mm = tv
	case multimsg.MultiMessage:
		mm = &tv
	case margaret.SeqWrapper:
		wrappedVal := tv.Value()
		theMsg, ok := wrappedVal.(multimsg.MultiMessage)
		if !ok {
			return nil, fmt.Errorf("luigiUtils: expected MultiMessage in sequence wrapper - got %T", wrappedVal)
		}
		mm = &theMsg

	default:
		return nil, fmt.Errorf("luigiUtils: expected MultiMessage - got %T", val)
	}
	return mm, nil
}
