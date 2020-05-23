package threads

import (
	"go.cryptoscope.co/ssb"
	"go.cryptoscope.co/ssb/processing"
)

/*

thread extractor

threads: {
	foo: {
		root: %abc,
		previous: [ %def, ... ]
	},
	bar: {
		root: %abc,
		previous: [ %ghi, ... ]
	}
}

*/

func getRoot(m map[string]interface{}) string {
	root, _ := m["root"].(string)
	return root
}

func getPrevious(m map[string]interface{}) []string {
	var prevs []string

	iprevs, ok := m["previous"].([]interface{})
	if ok {
		prevs = make([]string, 0, len(iprevs))

		for _, iprev := range iprevs {
			if prev, ok := iprev.(string); ok {
				prevs = append(prevs, prev)
			}
		}
	}

	return prevs
}

func NewExtractor(rl rumourlog.Index) processing.MessageExtractor {
	entryXtr := func(m map[string]interface{}) []processing.KVPair {
		var root ssb.MessageRef
		err := root.Parse(getRoot(m))

	}

	return processing.MessageContentExtractor(
		processing.TraverseSelector([]string{},
			processing.RecurseSelector(1, func(m map[string]interface{}) []processing.KVPair {

			})))
}

/*
TODO: this should become a thing that somehow uses rumorlog sequence numbers
      when the key is a cipherlink. not sure about concrete layout yet.
func RefExtractor() ContentExtractor {
	return func(content map[string]interface{}) []KVPair {
		var kvs []KVPair

		for k, v := range content {
			str, ok := v.(string)
			if ok {
				kvs = append(kvs, KVPair{Key: []byte(k), Value: []byte(str)})
			}
		}

		return kvs
	}
}
*/
