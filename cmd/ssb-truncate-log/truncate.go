// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/ssbc/margaret"
	"github.com/ssbc/go-ssb/repo"

	"github.com/ssbc/go-luigi"
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
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: migrate3 <from> <to> <limit>")
		os.Exit(1)
	}
	fromPath := os.Args[1]
	toPath := os.Args[2]

	limit, err := strconv.Atoi(os.Args[3])
	check(err)

	repoFrom := repo.New(fromPath)
	repoTo := repo.New(toPath)

	from, err := repo.OpenLog(repoFrom)
	check(err)

	to, err := repo.OpenLog(repoTo)
	check(err)

	fmt.Println("element count in source log:", from.Seq())
	start := time.Now()
	src, err := from.Query(margaret.Limit(limit))
	check(err)
	err = src.(luigi.PushSource).Push(context.TODO(), luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err == (luigi.EOS{}) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("push failed: %w", err)
		}

		if err, ok := v.(error); ok {
			if margaret.IsErrNulled(err) {
				return nil
			}
			return err
		}

		seq, err := to.Append(v)
		fmt.Print("\r", seq)
		return err
	}))
	check(err)
	fmt.Println()
	fmt.Println("copy done after:", time.Since(start))

	fmt.Println("target has", to.Seq())
}
