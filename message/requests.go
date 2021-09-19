// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package message

import (
	"bytes"
	"encoding/json"
	"fmt"

	refs "go.mindeco.de/ssb-refs"
)

// TODO: these might besser be moved to some kind of "rpc replies and arguments types" package

// WhoamiReply is the reply from a whoami muxrpc call
type WhoamiReply struct {
	ID refs.FeedRef `json:"id"`
}

// CommonArgs defines common arguments for query rpc calls
type CommonArgs struct {
	// Keys should the value be wrapped in it's key or not?
	// can't omit this falsy value, the JS-stack stack assumes true if it's not there
	Keys bool `json:"keys"`

	Values bool `json:"values,omitempty"`

	// Live controls wether the query should be kept open to wait for future values
	Live bool `json:"live,omitempty"`

	// Private controls wether the query should contain private messages
	Private bool `json:"private,omitempty"`
}

// StreamArgs defines common arguments for stream rpc calls
type StreamArgs struct {
	// Limit after how many elements the stream should end.
	// Main gotcha here is that limit:0 is treated literally as "empty stream".
	// Remember to set it to -1 for infinite or use NewStreamArgs().
	Limit int64 `json:"limit,omitempty"`

	Gt RoundedInteger `json:"gt,omitempty"`
	Lt RoundedInteger `json:"lt,omitempty"`

	Reverse bool `json:"reverse,omitempty"`
}

// NewStreamArgs returns a StreamArgs initialzed with limit:-1
func NewStreamArgs() StreamArgs {
	return StreamArgs{
		Limit: -1,
	}
}

// CreateHistArgs defines the query parameters for the createHistoryStream rpc call
type CreateHistArgs struct {
	CommonArgs
	StreamArgs

	ID  refs.FeedRef `json:"id,omitempty"`
	Seq int64        `json:"seq,omitempty"`

	AsJSON bool `json:"asJSON,omitempty"`
}

// NewCreateHistoryStreamArgs returns CreateHistArgs but with NewStreamArgs() (limit set to -1)
func NewCreateHistoryStreamArgs() CreateHistArgs {
	return CreateHistArgs{
		StreamArgs: NewStreamArgs(),
	}
}

// CreateLogArgs defines the query parameters for the createLogStream rpc call
type CreateLogArgs struct {
	CommonArgs
	StreamArgs

	Seq int64 `json:"seq"`
}

// MessagesByTypeArgs defines the query parameters for the messagesByType rpc call
type MessagesByTypeArgs struct {
	CommonArgs
	StreamArgs

	Type string `json:"type"`
}

// TanglesArgs specifies the root of the thread to read.
type TanglesArgs struct {
	CommonArgs
	StreamArgs

	Root refs.MessageRef `json:"root"`

	// indicate the v2 subtangle (group, ...)
	// empty string for v1 tangle
	Name string `json:"name"`
}

// RoundedInteger also accepts unmarshaling from a float
type RoundedInteger int64

// UnmarshalJSON turns a float into a truncated integer
func (ri *RoundedInteger) UnmarshalJSON(input []byte) error {
	var isFloat = false
	if idx := bytes.Index(input, []byte(".")); idx > 0 {
		input = input[:idx]
		isFloat = true
	}

	var i int64
	err := json.Unmarshal(input, &i)
	if err != nil {
		return fmt.Errorf("RoundedInteger: input is not an int: %w", err)
	}

	*ri = RoundedInteger(i)
	if isFloat {
		*ri++
	}

	return nil
}
