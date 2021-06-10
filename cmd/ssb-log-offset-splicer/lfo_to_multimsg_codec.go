// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"go.cryptoscope.co/margaret"
	refs "go.mindeco.de/ssb-refs"
)

type FlumeToMultiMsgCodec struct{}

var _ margaret.Codec = (*FlumeToMultiMsgCodec)(nil)

func (c FlumeToMultiMsgCodec) Marshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := c.NewEncoder(&buf)
	err := enc.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("bytes codec: encode failed: %w", err)
	}
	return buf.Bytes(), nil
}

func (c FlumeToMultiMsgCodec) Unmarshal(data []byte) (interface{}, error) {
	dec := c.NewDecoder(bytes.NewReader(data))

	return dec.Decode()
}

type testEncoder struct{ w io.Writer }

func (te testEncoder) Encode(v interface{}) error {
	lfoM, ok := v.(lfoMessage)
	if !ok {
		return fmt.Errorf("can only write bytes (not %T)", v)
	}

	_, err := te.w.Write(lfoM.raw)
	return err
}

func (c FlumeToMultiMsgCodec) NewEncoder(w io.Writer) margaret.Encoder {
	return testEncoder{w: w}
}

func (c FlumeToMultiMsgCodec) NewDecoder(r io.Reader) margaret.Decoder {
	return &decoder{r: r}
}

type decoder struct{ r io.Reader }

func (dec *decoder) Decode() (interface{}, error) {

	var m lfoMessage

	var err error
	m.raw, err = ioutil.ReadAll(dec.r)
	if err != nil {
		return nil, err
	}

	var justTheAuthor struct {
		Value struct {
			Author refs.FeedRef
		}
	}
	err = json.Unmarshal(m.raw, &justTheAuthor)
	if err != nil {
		return nil, err
	}
	m.author = justTheAuthor.Value.Author

	return m, nil
}

type lfoMessage struct {
	raw []byte

	author refs.FeedRef
}
