// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"encoding/base64"
	"fmt"

	"go.mindeco.de/log"
)

// Option allows to tune certain aspects of a client
type Option func(*Client) error

// WithContext sets a specific context (for cancellation and timeouts).
// Otherwise context.Background() is used.
func WithContext(ctx context.Context) Option {
	return func(c *Client) error {
		c.rootCtx = ctx
		return nil
	}
}

// WithLogger sets a different logger.
// By default the level.Info filter is applied.
func WithLogger(l log.Logger) Option {
	return func(c *Client) error {
		c.logger = l
		return nil
	}
}

// WithSHSAppKey changes the netcap/application key for secret-handshake
func WithSHSAppKey(appKey string) Option {
	return func(c *Client) error {
		var err error
		c.appKeyBytes, err = base64.StdEncoding.DecodeString(appKey)
		if err != nil {
			return fmt.Errorf("ssbClient: failed to decode secret-handshake appKey: %w", err)
		}
		if n := len(c.appKeyBytes); n != 32 {
			return fmt.Errorf("ssbClient: invalid length for appKey: %d", n)
		}
		return nil
	}
}
