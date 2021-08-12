// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package ctxutils

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.cryptoscope.co/luigi"
)

func TestWithErrContext(t *testing.T) {
	type testcase struct {
		closes []string
		expErr error
	}

	tcs := []testcase{
		{
			closes: []string{"cls"},
			expErr: luigi.EOS{},
		},
		{
			closes: []string{"cancel"},
			expErr: context.Canceled,
		},
		{
			closes: []string{"cls", "cancel"},
			expErr: luigi.EOS{},
		},
		{
			closes: []string{"cancel", "cls"},
			expErr: context.Canceled,
		},
		{
			expErr: nil,
		},
	}

	mkTest := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			ctx := context.Background()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			ctx, cls := WithError(ctx, luigi.EOS{})
			defer cls()

			for _, op := range tc.closes {
				switch op {
				case "cls":
					cls()
				case "cancel":
					cancel()
				default:
					t.Error("unexpected element in closes:", op)
				}

				// give other goroutine some time
				time.Sleep(time.Millisecond)
			}

			if ctx.Err() != tc.expErr {
				t.Errorf("error mismatch: expected %q, got: %v", tc.expErr, ctx.Err())
			}
		}
	}

	for i, tc := range tcs {
		t.Run(fmt.Sprintf("%d-%s", i, strings.Join(tc.closes, ",")), mkTest(tc))
	}
}
