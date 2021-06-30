// SPDX-License-Identifier: MIT

package message

import (
	"fmt"

	refs "go.mindeco.de/ssb-refs"
)

type WhoamiReply struct {
	ID refs.FeedRef `json:"id"`
}

type CommonArgs struct {
	Keys   bool `json:"keys"` // can't omit this falsy value, the JS-stack stack assumes true if it's not there
	Values bool `json:"values,omitempty"`
	Live   bool `json:"live,omitempty"`

	Private bool `json:"private,omitempty"`
}

type StreamArgs struct {
	Limit MargaretLimit `json:"limit,omitempty"`

	Gt int64 `json:"gt,omitempty"`
	Lt int64 `json:"lt,omitempty"`

	Reverse bool `json:"reverse,omitempty"`
}

// var _ json.Unmarshaler = (*MargaretLimit)(nil)

type MargaretLimit int64

func (ml MargaretLimit) Int() int64 {
	fmt.Println("returning ", ml)
	return int64(ml)
}

func (ml *MargaretLimit) UnmarshalText(data []byte) error {
	// func (ml *MargaretLimit) UnmarshalJSON(data []byte) error {
	fmt.Println("received input ", string(data))
	return fmt.Errorf("TODO: %q", string(data))
}

// CreateHistArgs defines the query parameters for the createHistoryStream rpc call
type CreateHistArgs struct {
	CommonArgs
	StreamArgs

	Limit MargaretLimit `json:"limit,omitempty"`

	ID  refs.FeedRef `json:"id,omitempty"`
	Seq int64        `json:"seq,omitempty"`

	AsJSON bool `json:"asJSON,omitempty"`
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

type TanglesArgs struct {
	CommonArgs
	StreamArgs

	Root refs.MessageRef `json:"root"`

	// indicate the v2 subtangle (group, ...)
	// empty string for v1 tangle
	Name string `json:"name"`
}
