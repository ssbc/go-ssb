package repo

import (
	"context"

	"go.cryptoscope.co/sbot"
)

type Option func(r *repo) error

func SetContext(ctx context.Context) Option {
	return func(r *repo) error {
		r.ctx = ctx
		return nil
	}
}

func SetKeyPair(kp *sbot.KeyPair) Option {
	return func(r *repo) error {
		r.keyPair = kp
		return nil
	}
}
