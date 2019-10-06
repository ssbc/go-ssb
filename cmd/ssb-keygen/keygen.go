// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/pkg/errors"
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/repo"
)

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %+v\n", err)
	fmt.Fprintln(os.Stderr, "occurred at")
	// debug.PrintStack()
	os.Exit(1)
}

var (
	repoDir  string
	feedAlgo string
)

func init() {
	u, err := user.Current()
	check(err)

	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to store the key")
	flag.StringVar(&feedAlgo, "format", ssb.RefAlgoFeedSSB1, "format to use")

	flag.Parse()

}

func main() {

	args := flag.Args()

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s (-format=algo, -repo=location) <name>", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	if feedAlgo != ssb.RefAlgoFeedSSB1 && feedAlgo != ssb.RefAlgoFeedGabby { //  enums would be nice
		check(errors.Errorf("invalid feed refrence algo. %s or %s", ssb.RefAlgoFeedSSB1, ssb.RefAlgoFeedGabby))
	}

	r := repo.New(repoDir)

	kp, err := repo.NewKeyPair(r, args[0], feedAlgo)
	check(err)

	fmt.Println(kp.Id.Ref())
}
