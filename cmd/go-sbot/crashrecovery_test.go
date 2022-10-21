// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ssbc/go-ssb/client"
	"github.com/ssbc/go-ssb/internal/testutils"
)

// make sure the process has an effective locking mechanism for the repo
func TestDontStartTwiceOnTheSameRepo(t *testing.T) {
	if testutils.SkipOnCI(t) {
		return
	}

	r := require.New(t)

	testPath := filepath.Join(".", "testrun", t.Name())
	r.NoError(os.RemoveAll(testPath))
	r.NoError(os.MkdirAll(testPath, 0700))

	binName := "go-sbot-testing"
	binPath := filepath.Join(testPath, binName)

	goBuild := exec.Command("go", "build", "-o", binPath)
	out, err := goBuild.CombinedOutput()
	r.NoError(err, "build command failed: %s", string(out))

	// TODO: rand hmac and shscap?
	bot1 := exec.Command(binPath, "-lis", ":0", "-repo", testPath)
	bot1.Stderr = os.Stderr
	bot1.Stdout = os.Stderr

	r.NoError(bot1.Start())

	// this shouldn't start
	bot2 := exec.Command(binPath, "-lis", ":0", "-repo", testPath)
	bot2.Stderr = os.Stderr
	bot2.Stdout = os.Stderr

	out, err = bot2.CombinedOutput()
	r.Error(err, "bot2 should have failed: %s", string(out))

	// "normal" shutdown
	out, err = exec.Command("kill", strconv.Itoa(bot1.Process.Pid)).CombinedOutput()
	r.NoError(err, "kill command failed: %s", string(out))
	err = bot1.Wait()
	r.Error(err)
}

func TestRecoverFromCrash(t *testing.T) {
	ctx := context.Background()

	r := require.New(t)

	testPath := filepath.Join(".", "testrun", t.Name())
	r.NoError(os.RemoveAll(testPath))
	r.NoError(os.MkdirAll(testPath, 0700))

	binName := "go-sbot-testing"
	binPath := filepath.Join(testPath, binName)

	goBuild := exec.Command("go", "build", "-o", binPath)
	out, err := goBuild.CombinedOutput()
	r.NoError(err, "build command failed: %s", string(out))

	// TODO: rand hmac and shscap?
	goBot := exec.Command(binPath, "-lis", ":0", "-repo", testPath)
	goBot.Stderr = os.Stderr
	goBot.Stdout = os.Stderr

	r.NoError(goBot.Start())

	try := 0
	checkSbot := func() {
		var i int
		for i = 10; i > 0; i-- {
			time.Sleep(250 * time.Millisecond)

			c, err := client.NewUnix(filepath.Join(testPath, "socket"), client.WithContext(ctx))
			if err != nil && i > 0 {
				t.Logf("%d: unable to make client", try)
				continue
			} else {
				r.NoError(err)
			}

			who, err := c.Whoami()
			if err != nil && i > 0 {
				t.Log("unable to call whoami")
				continue
			} else if err == nil {

			} else {
				r.NoError(err)
				r.NotNil(who)
				break
			}

			ref, err := c.Publish(struct {
				Type   string `json:"type"`
				Test   string
				Try, I int
			}{"test", "working!", try, i})
			r.NoError(err)
			t.Logf("%d:connection established (i:%d) %s", try, i, ref.String())

			c.Close()
			break
		}
		if i == 0 {
			t.Errorf("%d: check Sbot failed", try)
		}
		try++
	}
	checkSbot()

	// make it exit immediatly with goroutine dump
	out, err = exec.Command("kill", "-3", strconv.Itoa(goBot.Process.Pid)).CombinedOutput()
	r.NoError(err, "kill command failed: %s", string(out))

	err = goBot.Wait()
	r.NotNil(err)

	// should start up cleanly again
	goBot = exec.Command(binPath, "-lis", ":0", "-repo", testPath)
	goBot.Stderr = os.Stderr
	goBot.Stdout = os.Stderr

	r.NoError(goBot.Start())
	checkSbot()

	// "normal" shutdown
	out, err = exec.Command("kill", strconv.Itoa(goBot.Process.Pid)).CombinedOutput()
	r.NoError(err, "kill command failed: %s", string(out))
	r.Nil(goBot.Wait())
	// if err != nil {
	// 	execErr, ok := err.(*exec.ExitError)
	// 	r.True(ok, "err:%T", err)
	// 	// r.Equal(true, execErr.Exited())
	// 	r.Equal(true, execErr.Success(), "did not exit in success")
	// 	r.Equal(-1, execErr.ExitCode())
	// }

}
