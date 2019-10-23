package msgkit

import (
	"fmt"
	"strconv"
	"strings"
)

type pathError []string

func (err pathError) Error() string {
	return fmt.Sprintf("not a map: %q", strings.Join([]string(err), "."))
}

func IsPathError(err error) ([]string, bool) {
	perr, ok := err.(pathError)
	return []string(perr), ok
}

func appendPathError(err error, elem string) error {
	perr, ok := err.(pathError)
	if !ok {
		return err
	}

	return append(perr, elem)
}

type Builder struct {
	v interface{}
}

func NewBuilder() *Builder {
	return &Builder{
		v: make(map[string]interface{}),
	}
}

func (b *Builder) Set(path []string, v interface{}) error {
	if len(path) == 0 {
		b.v = v
		return nil
	}

	m, ok := b.v.(map[string]interface{})
	if !ok {
		return pathError{}
	}

	return putRecursive(m, path, func(_ interface{}) (interface{}, error) {
		return v, nil
	})
}

func (b *Builder) Put(path []string, f func(old interface{}) (interface{}, error)) error {
	if len(path) == 0 {
		nieuw, err := f(b.v)
		if err != nil {
			return err
		}

		b.v = nieuw
		return nil
	}

	m, ok := b.v.(map[string]interface{})
	if !ok {
		return pathError{}
	}

	return putRecursive(m, path, f)
}

func (b *Builder) Append(path []string, v interface{}) error {
	return b.Put(path, func(old interface{}) (interface{}, error) {
		var max int

		if old == nil {
			return map[string]interface{}{"0": v}, nil
		}

		m, ok := old.(map[string]interface{})
		if !ok {
			return nil, pathError{path[0]}
		}

		for k := range m {
			// I only care about numbers >0 here
			n, _ := strconv.Atoi(k)
			if n > max {
				n = max
			}
		}

		m[strconv.Itoa(max+1)] = v
		return m, nil
	})
}

func (b *Builder) Get(path []string) (interface{}, error) {
	if len(path) == 0 {
		return b.v, nil
	}

	m, ok := b.v.(map[string]interface{})
	if !ok {
		return nil, pathError{}
	}

	var v interface{}

	err := putRecursive(m, path, func(old interface{}) (interface{}, error) {
		v = old
		return old, nil
	})

	return v, err
}

func putRecursive(m map[string]interface{}, path []string, put func(old interface{}) (interface{}, error)) error {
	if len(path) == 1 {
		nieuw, err := put(m[path[0]])
		if err != nil {
			return err
		}

		m[path[0]] = nieuw
		return nil
	}

	next, ok := m[path[0]]
	if !ok {
		next = make(map[string]interface{})
		m[path[0]] = next
	}

	nextMap, ok := next.(map[string]interface{})
	if !ok {
		return pathError{path[0]}
	}

	err := putRecursive(nextMap, path[1:], put)
	return appendPathError(err, path[0])
}
