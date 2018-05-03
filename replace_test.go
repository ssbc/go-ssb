package ssb

import (
	"bytes"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/kylelemons/godebug/diff"
)

func TestUnicodeFind(t *testing.T) {
	str := []byte(`Test\u1234TEST\u4444`)
	matches := unicodeRegexp.FindAll(str, -1)
	if n := len(matches); n != 2 {
		t.Fatalf("expected 2 results, got %d", n)
	}
	if m := matches[0]; bytes.Compare(m, []byte("\\u1234")) != 0 {
		t.Errorf(`expected m[0] to be "1234" got "%s"`, m)
	}
	if m := matches[1]; bytes.Compare(m, []byte("\\u4444")) != 0 {
		t.Errorf(`expected m[1] to be "4444" got "%s"`, m)
	}
}

func TestUnicodeFindUpper(t *testing.T) {
	str := []byte(`wasn\u0027t`)
	matches := unicodeRegexp.FindAll(str, -1)
	if n := len(matches); n != 1 {
		t.Fatalf("expected 1 results, got %d", n)
	}
	if m := matches[0]; bytes.Compare(m, []byte("\\u0027")) != 0 {
		t.Errorf(`expected m[0] to be "1234" got "%s"`, m)
	}
}

func TestUnicodeReplace1(t *testing.T) {
	in := []byte(`wasn\u0027t`)
	want := []byte(`wasn't`)
	out := unicodeRegexp.ReplaceAllFunc(in, unicodeReplace)
	if bytes.Compare(want, out) != 0 {
		t.Error("output not as expected")
	}
	if d := diff.Diff(string(out), string(want)); len(d) != 0 && t.Failed() {
		t.Logf("\n%s", d)
	}
}
func TestUnicodeReplace2(t *testing.T) {
	in := []byte(`\U24B6\u2691`)
	want := []byte(`Ⓐ⚑`)
	out := unicodeRegexp.ReplaceAllFunc(in, unicodeReplace)
	if bytes.Compare(want, out) != 0 {
		t.Error("output not as expected")
	}
	if d := diff.Diff(string(out), string(want)); len(d) != 0 && t.Failed() {
		t.Logf("\n%s", d)
		for index, runeValue := range string(out) {
			t.Logf("%#U starts at byte position %d\n", runeValue, index)
		}
	}
}

func TestUnicodeReplace3(t *testing.T) {
	in := []byte(`\U0001f12f`)
	want := []byte(`🄯`)
	out := unicodeRegexp.ReplaceAllFunc(in, unicodeReplace)
	if bytes.Compare(want, out) != 0 {
		t.Error("output not as expected")
	}
	if d := diff.Diff(string(out), string(want)); len(d) != 0 && t.Failed() {
		t.Logf("\n%s", d)
		rv, width := utf8.DecodeRuneInString(string(want))
		t.Logf("Rune: %#U (W:%d)", rv, width)
		for i, r := range string(in) {
			fmt.Printf(" In:%d\t%#U\n", i, r)
		}
		for i, r := range string(out) {
			fmt.Printf("Out:%d\t%#U\n", i, r)
		}
	}
}
