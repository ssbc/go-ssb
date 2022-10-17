// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package sbot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	refs "go.mindeco.de/ssb-refs"

	"go.cryptoscope.co/ssb/internal/leakcheck"
	"go.cryptoscope.co/ssb/internal/testutils"
)

func TestPublishUnicode(t *testing.T) {
	if testutils.SkipOnCI(t) {
		// https://github.com/ssbc/go-ssb/pull/167
		return
	}

	defer leakcheck.Check(t)
	r := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())

	hk := make([]byte, 32)
	n, err := rand.Read(hk)
	r.Equal(32, n)
	os.RemoveAll("testrun")

	mainLog := testutils.NewRelativeTimeLogger(nil)
	ali, err := New(
		WithHMACSigning(hk),
		WithInfo(mainLog),
		WithRepoPath(filepath.Join("testrun", t.Name(), "ali")),
		WithListenAddr(":0"),
	)
	r.NoError(err)

	var aliErrc = make(chan error, 1)
	go func() {
		err := ali.Network.Serve(ctx)
		if err != nil && err != context.Canceled {
			aliErrc <- fmt.Errorf("ali serve exited: %w", err)
		}
		close(aliErrc)
	}()
	txt, err := hex.DecodeString("426c65657020626c6f6f702061696ee28099740a0a4e6f2066756e0a0af09f98a9")
	r.NoError(err)
	type post struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	newMsg := post{
		"post", string(txt),
	}
	_, err = ali.PublishLog.Append(newMsg)
	r.NoError(err)

	src, err := ali.ReceiveLog.Query()
	r.NoError(err)
	var i = 0
	for {
		v, err := src.Next(ctx)
		if luigi.IsEOS(err) {
			break
		}
		sm := v.(refs.Message)
		var p post
		c := sm.ContentBytes()
		err = json.Unmarshal(c, &p)
		r.NoError(err)
		r.Equal(newMsg.Text, p.Text)
		i++
	}
	r.Equal(i, 1)

	ali.Shutdown()
	cancel()
	r.NoError(ali.Close())
	r.NoError(<-aliErrc)
}
