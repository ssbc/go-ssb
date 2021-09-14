// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"encoding/json"
	"fmt"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/auth"
)

// MetafeedAnnounce is the type needed to do upgrades from existing classic feeds to the metafeed world.
// https://github.com/ssb-ngi-pointer/ssb-meta-feeds-spec#existing-ssb-identity
type MetafeedAnnounce struct {
	Type     string       `json:"type"`
	Subfeed  refs.FeedRef `json:"subfeed"`
	Metafeed refs.FeedRef `json:"metafeed"`

	Tangles refs.Tangles `json:"tangles"`
}

const metafeedAnnounceType = "metafeed/announce"

// Creates a fresh MetafeedAnnounce value with all the fields initialzed properly (especially Type and Tangles)
func NewMetafeedAnnounce(theMeta, theUpgrading refs.FeedRef) MetafeedAnnounce {
	var ma MetafeedAnnounce
	ma.Type = metafeedAnnounceType

	ma.Metafeed = theMeta
	ma.Subfeed = theUpgrading

	ma.Tangles = make(refs.Tangles)
	ma.Tangles["metafeed"] = refs.TanglePoint{Root: nil, Previous: nil}
	return ma
}

func (ma MetafeedAnnounce) Sign(priv ed25519.PrivateKey, hmacSecret *[32]byte) (json.RawMessage, error) {
	pp, err := jsonAndPreserve(ma)
	if err != nil {
		return nil, fmt.Errorf("legacySign: error during sign prepare: %w", err)
	}

	if hmacSecret != nil {
		mac := auth.Sum(pp, hmacSecret)
		pp = mac[:]
	}

	sig := ed25519.Sign(priv, pp)

	var signedMsg signedMetafeedAnnouncment
	signedMsg.MetafeedAnnounce = ma
	signedMsg.Signature = EncodeSignature(sig)

	return json.Marshal(signedMsg)
}

// signedMetafeedAnnouncment wrapps a MetafeedAnnounce with a Signature
type signedMetafeedAnnouncment struct {
	MetafeedAnnounce

	Signature Signature `json:"signature"`
}

// VerifyMetafeedAnnounce takes a raw json body and asserts the validity of the signature and that it is for the right feed.
func VerifyMetafeedAnnounce(data []byte, subfeedAuthor refs.FeedRef, hmacSecret *[32]byte) (MetafeedAnnounce, bool) {
	var sma signedMetafeedAnnouncment
	err := json.Unmarshal(data, &sma)
	if err != nil {
		return MetafeedAnnounce{}, false
	}

	if sma.Type != metafeedAnnounceType {
		return MetafeedAnnounce{}, false
	}

	if !sma.Subfeed.Equal(subfeedAuthor) {
		return MetafeedAnnounce{}, false
	}

	pp, err := jsonAndPreserve(sma)
	if err != nil {
		return MetafeedAnnounce{}, false
	}

	rest, sig, err := ExtractSignature(pp)
	if err != nil {
		return MetafeedAnnounce{}, false
	}

	if hmacSecret != nil {
		mac := auth.Sum(rest, hmacSecret)
		rest = mac[:]
	}

	err = sig.Verify(rest, sma.Metafeed)
	if err != nil {
		return MetafeedAnnounce{}, false
	}

	return sma.MetafeedAnnounce, true
}
