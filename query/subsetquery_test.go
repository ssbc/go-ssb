// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package query_test

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"

	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/query"
	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/sbot"
	refs "go.mindeco.de/ssb-refs"
)

func TestSubsetQuerySerializing(t *testing.T) {
	testRef, err := refs.NewFeedRefFromBytes(bytes.Repeat([]byte{1}, 32), refs.RefAlgoFeedSSB1)
	if err != nil {
		t.Fatal(err)
	}

	cases := []tcaseSerialized{
		{
			name:      "simple type",
			query:     query.NewSubsetOpByType("foo"),
			jsonInput: `{"op":"type","string":"foo"}`,
		},

		{
			name:      "simple author",
			query:     query.NewSubsetOpByAuthor(testRef),
			jsonInput: `{"op":"author","feed":"@AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=.ed25519"}`,
		},

		{
			name: "simple and",
			query: query.NewSubsetAndCombination(
				query.NewSubsetOpByAuthor(testRef),
				query.NewSubsetOpByType("foo"),
			),
			jsonInput: `{"op":"and","args":[{"op":"author","feed":"@AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=.ed25519"},{"op":"type","string":"foo"}]}`,
		},

		{
			name: "simple or",
			query: query.NewSubsetOrCombination(
				query.NewSubsetOpByType("foo"),
				query.NewSubsetOpByType("bar"),
			),
			jsonInput: `{"op":"or","args":[{"op":"type","string":"foo"},{"op":"type","string":"bar"}]}`,
		},

		{
			name: "author and two types",
			query: query.NewSubsetAndCombination(
				query.NewSubsetOpByAuthor(testRef),
				query.NewSubsetOrCombination(
					query.NewSubsetOpByType("foo"),
					query.NewSubsetOpByType("bar"),
				),
			),
			jsonInput: `{"op":"and","args":[{"op":"author","feed":"@AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=.ed25519"},{"op":"or","args":[{"op":"type","string":"foo"},{"op":"type","string":"bar"}]}]}`,
		},

		{
			name:      "invalid operation",
			jsonInput: `{"op":"stuff","times":"over 9000"}`,
			invalid:   true,
		},

		{
			name:      "invalid feed",
			jsonInput: `{"op":"author","feed":"over 9000"}`,
			invalid:   true,
		},

		{
			name:      "empty feed",
			jsonInput: `{"op":"author","feed":""}`,
			invalid:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

type tcaseSerialized struct {
	name string

	jsonInput string
	query     query.SubsetOperation

	invalid bool
}

func (tc tcaseSerialized) run(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	var parsed query.SubsetOperation
	err := json.Unmarshal([]byte(tc.jsonInput), &parsed)
	if tc.invalid {
		r.Error(err, "expected error parsing invalid input")
		return
	}

	r.NoError(err, "error while parsing json input")

	a.Equal(tc.query, parsed, "generated output does not map back to input")

	out, err := json.Marshal(tc.query)
	r.NoError(err, "failed to create json from input query")

	a.Equal(tc.jsonInput, string(out), "failed to create the wanted output")
}

func TestSubsetQueryPlanExecution(t *testing.T) {
	r := require.New(t)

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.NoError(err)
	r.Equal(32, n)

	tRepoPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(tRepoPath)

	tRepo := repo.New(tRepoPath)

	// make three new keypairs with nicknames
	n2kp := make(map[string]ssb.KeyPair)

	kpArny, err := repo.NewKeyPair(tRepo, "arny", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["arny"] = kpArny

	kpBert, err := repo.NewKeyPair(tRepo, "bert", refs.RefAlgoFeedGabby)
	r.NoError(err)
	n2kp["bert"] = kpBert

	kpCloe, err := repo.NewKeyPair(tRepo, "cloe", refs.RefAlgoFeedSSB1)
	r.NoError(err)
	n2kp["cloe"] = kpCloe

	kps, err := repo.AllKeyPairs(tRepo)
	r.NoError(err)
	r.Len(kps, 3)

	// make the bot
	logger := log.NewLogfmtLogger(os.Stderr)
	mainbot, err := sbot.New(
		sbot.WithInfo(logger),
		sbot.WithRepoPath(tRepoPath),
		sbot.WithHMACSigning(hk),
		sbot.DisableNetworkNode(),
	)
	r.NoError(err)

	// create some messages
	var testRefs []refs.MessageRef
	testMsgs := []struct {
		as string      // nick name
		c  interface{} // content
	}{
		{"arny", refs.NewAboutName(kpArny.ID(), "i'm arny!")},
		{"arny", refs.NewContactFollow(kpBert.ID())},
		{"bert", refs.NewAboutName(kpBert.ID(), "i'm bert!")},
		{"bert", refs.NewContactFollow(kpArny.ID())},
		{"bert", refs.NewAboutName(kpCloe.ID(), "that cloe")},
		{"cloe", refs.NewAboutName(kpBert.ID(), "iditot")},
		{"cloe", refs.NewPost("hello, world!")},
		{"cloe", refs.NewAboutName(kpCloe.ID(), "i'm cloe!")},
	}

	for idx, intro := range testMsgs {
		ref, err := mainbot.PublishAs(intro.as, intro.c)
		r.NoError(err, "publish %d failed", idx)
		r.NotNil(ref)
		testRefs = append(testRefs, ref.Key())
	}

	r.EqualValues(len(testMsgs)-1, mainbot.ReceiveLog.Seq(), "did not get all the messages")

	sp := query.NewSubsetPlaner(mainbot.Users, mainbot.ByType)

	t.Run("by author", func(t *testing.T) {
		r := require.New(t)

		qry := query.NewSubsetOpByAuthor(kpArny.ID())

		msgs, err := sp.QuerySubsetMessages(mainbot.ReceiveLog, qry)
		r.NoError(err)
		r.Len(msgs, 2, "wrong number of resulting messages")
		msgRefs := messagesToRefs(msgs)
		r.True(testRefs[0].Equal(msgRefs[0]))
		r.True(testRefs[1].Equal(msgRefs[1]))
	})

	t.Run("by type", func(t *testing.T) {
		r := require.New(t)

		qry := query.NewSubsetOpByType("post")

		msgs, err := sp.QuerySubsetMessages(mainbot.ReceiveLog, qry)
		r.NoError(err)
		r.Len(msgs, 1, "wrong number of resulting messages")
		msgRefs := messagesToRefs(msgs)
		r.True(testRefs[6].Equal(msgRefs[0]))
	})

	t.Run("OR two types (contact and post)", func(t *testing.T) {
		r := require.New(t)

		qry := query.NewSubsetOrCombination(
			query.NewSubsetOpByType("contact"),
			query.NewSubsetOpByType("post"),
		)

		msgs, err := sp.QuerySubsetMessages(mainbot.ReceiveLog, qry)
		r.NoError(err)
		r.Len(msgs, 3, "wrong number of resulting messages")
		msgRefs := messagesToRefs(msgs)
		r.True(testRefs[1].Equal(msgRefs[0]))
		r.True(testRefs[3].Equal(msgRefs[1]))
		r.True(testRefs[6].Equal(msgRefs[2]))
	})

	// shutdown bot
	mainbot.Shutdown()
	r.NoError(mainbot.Close())
}

func messagesToRefs(msgs []refs.Message) []refs.MessageRef {
	r := make([]refs.MessageRef, len(msgs))
	for i, m := range msgs {
		r[i] = m.Key()
	}
	return r
}
