// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/ssbc/margaret"
	"github.com/ssbc/go-ssb/message/multimsg"
	refs "github.com/ssbc/go-ssb-refs"
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
	bytes, ok := v.([]byte)
	if !ok {
		return fmt.Errorf("can only write bytes (not %T)", v)
	}

	_, err := te.w.Write(bytes)
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
	entry, err := ioutil.ReadAll(dec.r)
	if err != nil {
		return nil, err
	}

	var msg refs.KeyValueRaw
	err = json.Unmarshal(entry, &msg)
	if err != nil {
		return nil, err
	}

	// we need to do two json decode passes here.
	// if we try to convert `value: { ... }` back from the decoded data, validation will fail.
	var onlyValue struct {
		// ignore key: (can use the decode above for that)
		Value json.RawMessage
	}
	err = json.Unmarshal(entry, &onlyValue)
	if err != nil {
		return nil, err
	}

	write := multimsg.NewMultiMessageFromKeyValRaw(msg, onlyValue.Value)
	return write, nil
}
