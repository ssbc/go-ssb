// Package invite contains functions for parsing invite codes and dialing a pub as a guest to redeem a token.
// The muxrpc handlers and storage are found in plugins/legacyinvite.
package invite

import (
	"bytes"
	"context"
	"log"

	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc/v2"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/client"
	refs "go.mindeco.de/ssb-refs"
)

// Redeem takes an invite token and a long term key.
// It uses the information in the token to build a guest-client connection
// and place an 'invite.use' rpc call with it's longTerm key.
// If the peer responds with a message it returns nil
func Redeem(ctx context.Context, tok Token, longTerm *refs.FeedRef) error {
	inviteKeyPair, err := ssb.NewKeyPair(bytes.NewReader(tok.Seed[:]))
	if err != nil {
		return errors.Wrap(err, "invite: couldn't make keypair from seed")
	}

	// now use the invite
	inviteClient, err := client.NewTCP(inviteKeyPair, tok.Address, client.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "invite: failed to establish guest-client connection")
	}

	var ret refs.KeyValueRaw
	var param = struct {
		Feed string `json:"feed"`
	}{longTerm.Ref()}

	err = inviteClient.Async(ctx, &ret, muxrpc.TypeJSON, muxrpc.Method{"invite", "use"}, param)
	if err != nil {
		return errors.Wrap(err, "invite: invalid token")
	}

	if ret.Key() != nil {
		log.Println("invite redeemed. Peer replied with msg", ret.Key().Ref())
	} else {
		log.Println("warning: peer replied with empty message")
	}

	inviteClient.Close()
	return nil
}
