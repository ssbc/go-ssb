package sbot

import "cryptoscope.co/go/muxrpc"

type Plugin interface {
	// Name returns the name and version of the plugin.
	// format: name-1.0.2
	Name() string

	// Handler returns the muxrpc handler for the plugin
	Handler(node Node) muxrpc.Handler

	// WrapEndpoint wraps the endpoint and returns something that has convenience wrappers.
	// The caller needs to type-assert the return value to something that is specific to the plugin.
	WrapEndpoint(edp muxrpc.Endpoint) interface{}
}
