package mfr // import "go.cryptoscope.co/luigi/mfr"

import (
	"context"

	"go.cryptoscope.co/luigi"
)

type FilterFunc func(context.Context, interface{}) (bool, error)

func SinkFilter(sink luigi.Sink, f FilterFunc) luigi.Sink {
	return &sinkFilter{
		Sink: sink,
		f:    f,
	}
}

type sinkFilter struct {
	luigi.Sink

	f FilterFunc
}

func (sink *sinkFilter) Pour(ctx context.Context, v interface{}) error {
	pass, err := sink.f(ctx, v)
	if err == nil && pass {
		err = sink.Sink.Pour(ctx, v)
	}

	return err
}

func SourceFilter(src luigi.Source, f FilterFunc) luigi.Source {
	return &srcFilter{
		Source: src,
		f:      f,
	}
}

type srcFilter struct {
	luigi.Source

	f FilterFunc
}

func (src *srcFilter) Next(ctx context.Context) (v interface{}, err error) {
	var pass bool

	for !pass {
		v, err = src.Source.Next(ctx)
		if err != nil {
			return nil, err
		}

		pass, err = src.f(ctx, v)
		if err != nil {
			return nil, err
		}
	}

	return v, nil
}
