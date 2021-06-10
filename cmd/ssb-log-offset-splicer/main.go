// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/legacyflumeoffset"
	"go.cryptoscope.co/margaret/offset2"
	"go.cryptoscope.co/ssb/message/multimsg"
)

func check(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

type FeedInfo struct {
	secret string
	ID     string
	log    margaret.Log
}

func mapIdentitiesToSecrets(indir, outdir, oformat string) map[string]FeedInfo {
	feeds := make(map[string]FeedInfo)
	// folderMap is used to marshal a json blob, mapping secret files to ids
	folderMap := make(map[string]string)
	counter := 0
	err := filepath.WalkDir(indir, func(path string, info fs.DirEntry, err error) error {
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), "secret") {
			file, err := os.Open(path)
			check(err)
			b, err := io.ReadAll(file)
			check(err)
			// save the name of the secret for this identity
			v := FeedInfo{secret: info.Name()}
			// load the secret & pick out its feed id
			err = json.Unmarshal(b, &v)
			check(err)
			dest := filepath.Join(outdir, fmt.Sprintf("puppet-%d", counter))
			counter++
			var logdest string
			// we want to open the correct file format depending on the oformat (either log folder, or a log.offset)
			switch oformat {
			case "offset2":
				logdest = filepath.Join(dest, "log")
			case "lfo":
				// first, create correct folder structure
				basedir := filepath.Join(dest, "flume")
				err := os.MkdirAll(basedir, os.ModePerm)
				check(err)
				// set log location
				logdest = filepath.Join(basedir, "log.offset")
			default:
				fmt.Println("what format is this?", oformat)
			}
			// open a margaret log for the specified output format
			v.log, err = openLogWithFormat(logdest, oformat)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create output log for %s: %s\n", v.secret, err)
				os.Exit(1)
			}
			feeds[v.ID] = v
			folderMap[info.Name()] = v.ID
			// copy the secret file to the outdir as well
			err = os.WriteFile(filepath.Join(dest, "secret"), b, 0600)
			check(err)
		}
		return nil
	})
	check(err)
	// write a json blob mapping the folders to identities
	// (we cant use ids as folder names as unix does not like base64's charset)
	b, err := json.MarshalIndent(folderMap, "", "  ")
	check(err)
	err = os.WriteFile(filepath.Join(outdir, "secret-ids.json"), b, 0644)
	check(err)
	return feeds
}

func main() {
	var oformat string = "lfo"
	flag.Func("of", "what format to use for the output", validateLogFormat(&oformat))

	var dryRun bool
	flag.BoolVar(&dryRun, "dry", false, "only output what it would do")
	var limit int
	flag.IntVar(&limit, "limit", -1, "how many entries to copy (defaults to unlimited)")
	flag.Parse()

	logPaths := flag.Args()
	if len(logPaths) != 2 {
		cmdName := os.Args[0]
		fmt.Fprintf(os.Stderr, "usage: %s <options> <path to ssb-fixtures folder> <output path>\n", cmdName)
		os.Exit(1)
	}

	if dryRun {
		fmt.Fprintf(os.Stderr, "would convert legacy flume offset to %s\n", oformat)
		fmt.Fprintf(os.Stderr, "locations %s to %s\n", logPaths[0], logPaths[1])
		return
	}

	var (
		err   error
		input margaret.Log
	)

	sourceFile := filepath.Join(logPaths[0], "flume", "log.offset")
	input, err = openLogWithFormat(sourceFile, "lfo")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}
	feeds := mapIdentitiesToSecrets(logPaths[0], logPaths[1], oformat)
	/*
	* given a ssb-fixtures directory, and its monolithic flume log legacy.offset (mfl)
	* 1. read all the secrets to figure out which authors exist
	* 2. for each discovered author create a key in a map[string]margaret.Log
	* 3. go through each message in the mfl and filter out the messages into the corresponding log of the map
	* 4. finally, create folders for each author, using the author's pubkey as directory name, and dump an (offset2 and) flo
	* version of their log.offset representation. inside each folder, dump the correct secret as well
	 */

	src, err := input.Query(margaret.Limit(limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create query on input log %s: %s\n", logPaths[0], err)
		os.Exit(1)
	}

	i := 0

	// input.Seq().Value()

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

		msg := v.(lfoMessage)
		i++

		// siphon out the author
		a, has := feeds[msg.author.Ref()]
		if !has {
			fmt.Fprintf(os.Stderr, "\rskipping: %d (author: %s)\n", i, msg.author.Ref())
			continue
		}

		fmt.Fprintf(os.Stderr, "\rcurrent: %d", i)

		// bb, err := msg.MarshalBinary()
		// check(err)
		// fmt.Println(bb, string(bb))
		_, err = a.log.Append(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to write entry to output log %s: %s\n", logPaths[1], err)
			os.Exit(1)
		}

	}

	fmt.Fprintln(os.Stderr, "all done. closing output log. Copied:", i)

	for _, a := range feeds {
		if c, ok := a.log.(io.Closer); ok {
			if err = c.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to close output log %s: %s\n", logPaths[1], err)
			}
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
