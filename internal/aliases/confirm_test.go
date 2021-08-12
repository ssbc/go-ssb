// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package aliases

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/ssb"
	refs "go.mindeco.de/ssb-refs"
)

func TestConfirmation(t *testing.T) {
	r := require.New(t)

	// this is our room, it's not a valid feed but thats fine for this test
	roomID, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("test"), 8), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// to make the test deterministic, decided by fair dice roll.
	seed := bytes.Repeat([]byte("yeah"), 8)
	// our user, who will sign the registration
	userKeyPair, err := ssb.NewKeyPair(bytes.NewReader(seed), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	// create and fill out the registration for an alias (in this case the name of the test)
	var valid Registration
	valid.RoomID = roomID
	valid.UserID = userKeyPair.ID()
	valid.Alias = t.Name()

	// internal function to create the registration string
	msg := valid.createRegistrationMessage()
	want := "=room-alias-registration:@dGVzdHRlc3R0ZXN0dGVzdHRlc3R0ZXN0dGVzdHRlc3Q=.ed25519:@Rt2aJrtOqWXhBZ5/vlfzeWQ9Bj/z6iT8CMhlr2WWlG4=.ed25519:TestConfirmation"
	r.Equal(want, string(msg))

	// create the signed confirmation
	confirmation := valid.Sign(userKeyPair.Secret())

	yes := confirmation.Verify()
	r.True(yes, "should be valid for this room and feed")

	// make up another id for the invalid test(s)

	otherID, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte("nope"), 8), refs.RefAlgoFeedSSB1)
	r.NoError(err)

	confirmation.RoomID = otherID
	yes = confirmation.Verify()
	r.False(yes, "should not be valid for another room")

	confirmation.RoomID = roomID // restore
	confirmation.UserID = otherID
	yes = confirmation.Verify()
	r.False(yes, "should not be valid for this room but another feed")

	// puncture the signature to emulate an invalid one
	confirmation.Signature[0] = confirmation.Signature[0] ^ 1

	yes = confirmation.Verify()
	r.False(yes, "should not be valid anymore")

}
