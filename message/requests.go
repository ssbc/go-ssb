// SPDX-License-Identifier: MIT

package message

import (
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

type streamArgs struct {
	Limit int64 `json:"limit,omitempty"`

	Gt int64 `json:"gt,omitempty"`
	Lt int64 `json:"lt,omitempty"`

	Reverse bool `json:"reverse,omitempty"`
}

func NewStreamArgs() streamArgs {
	return streamArgs{
		Limit: -1,
	}
}

// CreateHistArgs defines the query parameters for the createHistoryStream rpc call
type CreateHistArgs struct {
	CommonArgs
	streamArgs

	ID  refs.FeedRef `json:"id,omitempty"`
	Seq int64        `json:"seq,omitempty"`

	AsJSON bool `json:"asJSON,omitempty"`
}

func NewCreateHistoryStreamArgs() CreateHistArgs {
	return CreateHistArgs{
		streamArgs: NewStreamArgs(),
	}
}

// CreateLogArgs defines the query parameters for the createLogStream rpc call
type CreateLogArgs struct {
	CommonArgs
	streamArgs

	Seq int64 `json:"seq"`
}

// MessagesByTypeArgs defines the query parameters for the messagesByType rpc call
type MessagesByTypeArgs struct {
	CommonArgs
	streamArgs
	Type string `json:"type"`
}

type TanglesArgs struct {
	CommonArgs
	streamArgs

	Root refs.MessageRef `json:"root"`

	// indicate the v2 subtangle (group, ...)
	// empty string for v1 tangle
	Name string `json:"name"`
}
