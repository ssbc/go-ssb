package processing

import (
	"encoding/json"

	"go.cryptoscope.co/ssb"
)

// MessageExtractor extracts key valu pairs from an ssb.Message.
type MessageExtractor func(ssb.Message) []KVPair

// MessageExtractor extracts key valu pairs from a string-keyed map
// that contains the content of an ssb message.
type ContentExtractor func(map[string]interface{}) []KVPair

// MessageContentExtractor unmarshals the message content and passes
// it into the provided map extractors.
func MessageContentExtractor(xtrs ...ContentExtractor) MessageExtractor {
	return func(msg ssb.Message) []KVPair {
		var (
			err     error
			kvs     []KVPair
			content map[string]interface{}
		)

		err = json.Unmarshal(msg.ValueContent().Content, &content)
		if err != nil {
			return kvs
		}

		for _, xtr := range xtrs {
			kvs = append(kvs, xtr(content)...)
		}

		return kvs
	}
}

// NewTraverseSelector first traverses the map along the provided path
// and then calls the provided map extractor on the value at that position.
func TraverseSelector(path []string, xtr ContentExtractor) ContentExtractor {
	return func(content map[string]interface{}) []KVPair {
		var (
			v         interface{}
			ok        bool
			cur       = content
			remaining = path
		)

		for len(remaining) > 0 {
			v, ok = cur[remaining[0]]
			remaining = remaining[1:]
			if !ok {
				return nil
			}

			cur, ok = v.(map[string]interface{})
			if !ok {
				return nil
			}
		}

		return xtr(cur)
	}
}

// RecurseSelector applies the xtr map extractor recursively down the
// content map, up to depth maxDepth.
// Returns nil for maxDepth = 0 and the same as xtr itself for maxDepth = 1.
func RecurseSelector(maxDepth int, xtr ContentExtractor) ContentExtractor {
	var recurse func(int, map[string]interface{}) []KVPair

	recurse = func(depthLeft int, m map[string]interface{}) []KVPair {
		if depthLeft == 0 {
			return nil
		}

		var kvs = xtr(m)

		for _, v := range m {
			if next, ok := v.(map[string]interface{}); ok {
				kvs = append(kvs, recurse(depthLeft-1, next)...)
			}
		}

		return kvs
	}

	return func(m map[string]interface{}) []KVPair {
		return recurse(maxDepth, m)
	}
}

// StringsExtractor extracts all key value pairs from the content map
// where the value is a string of maximum length maxLen.
func StringsExtractor(maxLen int) ContentExtractor {
	return func(content map[string]interface{}) []KVPair {
		var kvs []KVPair

		for k, v := range content {
			str, ok := v.(string)
			if ok && len(str) > maxLen {
				kvs = append(kvs, KVPair{Key: []byte(k), Value: []byte(str)})
			}
		}

		return kvs
	}
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
