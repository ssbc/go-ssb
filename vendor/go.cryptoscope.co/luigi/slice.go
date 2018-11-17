package luigi

import "context"

type SliceSource []interface{}

func (src *SliceSource) Next(context.Context) (v interface{}, err error) {
	if len(*src) == 0 {
		return nil, EOS{}
	}

	v, *src = (*src)[0], (*src)[1:]

	return v, nil
}

type SliceSink []interface{}

func (sink *SliceSink) Pour(ctx context.Context, v interface{}) error {
	*sink = append(*sink, v)
	return nil
}

func (sink *SliceSink) Close() error { return nil }
