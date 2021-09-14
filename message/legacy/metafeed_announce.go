// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package legacy

import (
	"encoding/json"
	"fmt"
	"os"

	refs "go.mindeco.de/ssb-refs"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/auth"
)

type MetafeedAnnounce struct {
	Type     string       `json:"type"`
	Subfeed  refs.FeedRef `json:"subfeed"`
	Metafeed refs.FeedRef `json:"metafeed"`

	Tangles refs.Tangles `json:"tangles"`
}

const metafeedAnnounceType = "metafeed/announce"

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
	// flatten interface{} content value
	pp, err := jsonAndPreserve(ma)
	if err != nil {
		return nil, fmt.Errorf("legacySign: error during sign prepare: %w", err)
	}

	if hmacSecret != nil {
		mac := auth.Sum(pp, hmacSecret)
		pp = mac[:]
	}

	sig := ed25519.Sign(priv, pp)

	var signedMsg SignedMetafeedAnnouncment
	signedMsg.MetafeedAnnounce = ma
	signedMsg.Signature = EncodeSignature(sig)

	return json.Marshal(signedMsg)
	// return jsonAndPreserve(signedMsg)
}

type SignedMetafeedAnnouncment struct {
	MetafeedAnnounce

	Signature Signature `json:"signature"`
}

func VerifyMetafeedAnnounce(data []byte, hmacSecret *[32]byte) bool {
	var sma SignedMetafeedAnnouncment
	err := json.Unmarshal(data, &sma)
	if err != nil {
		fmt.Fprintln(os.Stderr, "unmarshal failed: ", err)
		return false
	}

	if sma.Type != metafeedAnnounceType {
		fmt.Fprintln(os.Stderr, "wrong type", sma.Type)
		return false
	}

	pp, err := jsonAndPreserve(sma)
	if err != nil {
		fmt.Fprintln(os.Stderr, "prettyprint failed: ", err)
		return false
	}

	rest, sig, err := ExtractSignature(pp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "extract failed: ", err)
		return false
	}

	if hmacSecret != nil {
		mac := auth.Sum(rest, hmacSecret)
		rest = mac[:]
	}

	err = sig.Verify(rest, sma.Metafeed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify failed: ", err)
		return false
	}

	return true
}
