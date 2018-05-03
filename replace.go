package ssb

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// TODO: only handles TWO byte escaped sequences
var unicodeRegexp = regexp.MustCompile(`\\[uU][0-9a-fA-F]{4,8}`)

func unicodeReplace(b []byte) []byte {
	low := bytes.ToLower(b)
	if !bytes.HasPrefix(low, []byte("\\u")) {
		return b
	}
	num, err := strconv.ParseInt(fmt.Sprintf("0x%s", low[2:]), 0, 32)
	if err != nil {
		fmt.Printf("WARNING: ssb unicodeReplace() failed: %s\n", err)
		return []byte{}
	}
	//fmt.Printf("(%s) Got Num:%d\n", b, num)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%c", num)
	return buf.Bytes()
}
