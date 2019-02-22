package mfr // import "go.cryptoscope.co/luigi/mfr"

import (
	"context"

	"go.cryptoscope.co/luigi"
)

type MapFunc func(context.Context, interface{}) (interface{}, error)

func SinkMap(sink luigi.Sink, f MapFunc) luigi.Sink {
	return &sinkMap{
		Sink: sink,
		f:    f,
	}
}

type sinkMap struct {
	luigi.Sink

	f MapFunc
}

func (sink *sinkMap) Pour(ctx context.Context, v interface{}) error {
	v, err := sink.f(ctx, v)
	if err != nil {
		return err
	}

	return sink.Sink.Pour(ctx, v)
}

func SourceMap(src luigi.Source, f MapFunc) luigi.Source {
	return &srcMap{
		Source: src,
		f:      f,
	}
}

type srcMap struct {
	luigi.Source

	f MapFunc
}

func (src *srcMap) Next(ctx context.Context) (interface{}, error) {
	v, err := src.Source.Next(ctx)
	if err != nil {
		return nil, err
	}

	return src.f(ctx, v)
}
