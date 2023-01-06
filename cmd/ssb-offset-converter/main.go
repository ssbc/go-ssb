// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ssbc/go-luigi"
	"github.com/ssbc/go-ssb/message/multimsg"
	"github.com/ssbc/margaret"
	"github.com/ssbc/margaret/legacyflumeoffset"
	"github.com/ssbc/margaret/offset2"
)

func main() {

	var iformat, oformat string = "offset2", "offset2"

	flag.Func("if", "what format to use for the input", validateLogFormat(&iformat))
	flag.Func("of", "what format to use for the output", validateLogFormat(&oformat))

	var dryRun bool
	flag.BoolVar(&dryRun, "dry", false, "only output what it would do")

	var limit int
	flag.IntVar(&limit, "limit", -1, "how many entries to copy (defaults to unlimited)")

	flag.Parse()

	logPaths := flag.Args()
	if len(logPaths) != 2 {
		cmdName := os.Args[0]
		fmt.Fprintf(os.Stderr, "usage: %s <options> <input path> <output path>\n", cmdName)
		os.Exit(1)
	}

	if iformat == oformat && limit == -1 {
		fmt.Fprintf(os.Stderr, "warning: nothing to do. exiting")
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "would convert %s to %s\n", iformat, oformat)
		fmt.Fprintf(os.Stderr, "locations %s to %s\n", logPaths[0], logPaths[1])
		return
	}

	var (
		err           error
		input, output margaret.Log
	)

	input, err = openLogWithFormat(logPaths[0], iformat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}

	output, err = openLogWithFormat(logPaths[1], oformat)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open output log %s: %s\n", logPaths[1], err)
		os.Exit(1)
	}

	src, err := input.Query(margaret.Limit(limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create query on input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}

	ctx := context.Background()
	for {
		v, err := src.Next(ctx)
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			fmt.Fprintf(os.Stderr, "failed to get log entry %s: %s\n", logPaths[0], err)
			os.Exit(1)
		}

		_, err = output.Append(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write entry to output log %s: %s\n", logPaths[1], err)
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr, "all done. closing output log.")

	if c, ok := output.(io.Closer); ok {
		if err = c.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close output log %s: %s\n", logPaths[1], err)
		}
	}
}

func validateLogFormat(flag *string) func(string) error {
	return func(input string) error {
		switch strings.ToLower(input) {
		case "offset2":
			*flag = "offset2"
			return nil
		case "lfo", "legacy", "dominic", "flumelog":
			*flag = "lfo"
			return nil
		default:
			return fmt.Errorf("unknown log format: %s", input)
		}
	}
}

func openLogWithFormat(path string, format string) (margaret.Log, error) {
	switch format {
	case "offset2":
		return offset2.Open(path, multimsg.MargaretCodec{})
	case "lfo":
		return legacyflumeoffset.Open(path, FlumeToMultiMsgCodec{})
	default:
		return nil, fmt.Errorf("unknown log format: %s", format)
	}
}
