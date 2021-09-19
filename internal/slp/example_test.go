// SPDX-FileCopyrightText: 2021 The Go-SSB Authors
//
// SPDX-License-Identifier: MIT

package slp_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"go.cryptoscope.co/ssb/internal/slp"
)

func ExampleEncode() {

	d := hex.Dumper(os.Stdout)
	buf := new(bytes.Buffer)

	w := io.MultiWriter(d, buf)

	var ex ExampleStruct
	ex.Foo = []byte("hello, world")
	ex.Bar = "well, yea.. just an example, you know?"

	out, err := ex.MarshalBinary()
	if err != nil {
		panic(err)
	}

	w.Write(out)

	var back ExampleStruct
	err = back.UnmarshalBinary(buf.Bytes())
	if err != nil {
		panic(err)
	}

	if !bytes.Equal(ex.Foo, back.Foo) {
		fmt.Println("\n\tFoo is wrong")
	}

	if ex.Bar != back.Bar {
		fmt.Println("\n\tBar is wrong")
	}

	// Output:
	// 00000000  0c 00 68 65 6c 6c 6f 2c  20 77 6f 72 6c 64 26 00  |..hello, world&.|
	// 00000010  77 65 6c 6c 2c 20 79 65  61 2e 2e 20 6a 75 73 74  |well, yea.. just|
	// 00000020  20 61 6e 20 65 78 61 6d  70 6c 65 2c 20 79 6f 75  | an example, you|
	// 00000030  20 6b 6e 6f 77 3f
}

type ExampleStruct struct {
	Foo []byte
	Bar string
}

func (es ExampleStruct) MarshalBinary() ([]byte, error) {
	return slp.Encode(es.Foo, []byte(es.Bar))
}

func (es *ExampleStruct) UnmarshalBinary(in []byte) error {
	slices := slp.Decode(in)
	if len(slices) != 2 {
		return fmt.Errorf("expected two chunks")
	}

	es.Foo = slices[0]
	es.Bar = string(slices[1])

	return nil
}
