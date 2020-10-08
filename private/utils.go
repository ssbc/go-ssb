package private

import "sort"

// bytesSlice attaches the methods of sort.Interface to [][]byte, sorting in increasing order.
type bytesSlice [][]byte

func (p bytesSlice) Len() int { return len(p) }
func (p bytesSlice) Less(i, j int) bool {
	for k := range p[i] {
		if p[i][k] < p[j][k] {
			return true
		}
	}

	return false
}
func (p bytesSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

// Sort is a convenience method.
func (p bytesSlice) Sort() { sort.Sort(p) }

func sortAndConcat(bss ...[]byte) []byte {
	bytesSlice(bss).Sort()

	var l int
	for _, bs := range bss {
		l += len(bs)
	}

	var (
		buf = make([]byte, l)
		off int
	)

	for _, bs := range bss {
		off += copy(buf[off:], bs)
	}

	return buf
}
