// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/cryptix/go/logging"

	"github.com/pkg/errors"

	"go.cryptoscope.co/ssb/repo"
	"go.cryptoscope.co/ssb/repo/migrations"
	"go.cryptoscope.co/ssb/sbot"
)

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %s\n", err)
	fmt.Fprintln(os.Stderr, "occurred at")
	debug.PrintStack()
	os.Exit(1)
}

func main() {
	logging.SetupLogging(nil)
	logger := logging.Logger("migrate")
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: migrate2 <repo>")
		os.Exit(1)
	}
	repoDir := os.Args[1]

	repo := repo.New(repoDir)
	didUpgrade, err := migrations.UpgradeToMultiMessage(logger, repo)
	check(errors.Wrap(err, "BotInit: repo migration failed"))

	if didUpgrade {
		os.RemoveAll(repo.GetPath("indexes"))
		os.RemoveAll(repo.GetPath("sublogs"))

		sbot, err := sbot.New(
			sbot.WithInfo(logger),
			sbot.WithRepoPath(repoDir),
			sbot.DisableNetworkNode(),
			sbot.DisableLiveIndexMode())
		check(errors.Wrap(err, "BotInit: failed to make reindexing sbot"))
		err = sbot.Close()
		check(err)
	}
}
