// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"go.cryptoscope.co/ssb/repo"
	refs "go.mindeco.de/ssb-refs"
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
	feedAlgo = refs.RefAlgoFeedSSB1
)

func init() {
	u, err := user.Current()
	check(err)

	flag.StringVar(&repoDir, "repo", filepath.Join(u.HomeDir, ".ssb-go"), "where to store the key")
	flag.Func("format", "format to use", func(val string) error {

		candidate := refs.RefAlgo(val)

		if err := isValidFormat(candidate); err != nil {
			return err
		}

		return nil
	})

	flag.Parse()

}

func main() {

	args := flag.Args()

	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: %s (-format=algo, -repo=location) <name>", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	r := repo.New(repoDir)

	kp, err := repo.NewKeyPair(r, args[0], feedAlgo)
	check(err)

	fmt.Println(kp.ID().String())
}

func isValidFormat(f refs.RefAlgo) error {
	//  enums would be nice
	if f != refs.RefAlgoFeedSSB1 && f != refs.RefAlgoFeedGabby {
		return fmt.Errorf("invalid feed refrence algo. %s or %s", refs.RefAlgoFeedSSB1, refs.RefAlgoFeedGabby)
	}
	return nil
}
