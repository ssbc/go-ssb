package msgkit

import (
	"testing"

	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
)

type opPut struct {
	Builder *Builder
	Path    []string
	Func    func(old interface{}) (new interface{}, err error)

	ExpErr string
}

func (op opPut) Do(t *testing.T, env interface{}) {
	err := op.Builder.Put(op.Path, op.Func)
	if op.ExpErr == "" {
		require.NoError(t, err, "put")
	} else {
		require.EqualError(t, err, op.ExpErr, "put")
	}
}

type opSet struct {
	Builder *Builder
	Path    []string
	Value   interface{}

	ExpErr string
}

func (op opSet) Do(t *testing.T, env interface{}) {
	err := op.Builder.Set(op.Path, op.Value)
	if op.ExpErr == "" {
		require.NoError(t, err, "set")
	} else {
		require.EqualError(t, err, op.ExpErr, "set")
	}
}

type opGet struct {
	Builder *Builder
	Path    []string

	ExpValue interface{}
	ExpErr   string
}

func (op opGet) Do(t *testing.T, env interface{}) {
	v, err := op.Builder.Get(op.Path)
	if op.ExpErr == "" {
		require.NoError(t, err, "get")
		require.Equal(t, op.ExpValue, v, "get")
	} else {
		require.EqualError(t, err, op.ExpErr, "get")
	}
}

func TestBuilder(t *testing.T) {
	b := NewBuilder()

	tcs := []testops.TestCase{
		{
			Name: "set, update, read value",
			Ops: []testops.Op{
				opSet{
					Builder: b,
					Path:    []string{"type"},
					Value:   "post",
				},
				opPut{
					Builder: b,
					Path:    []string{"type"},
					Func: func(v interface{}) (interface{}, error) {
						str := v.(string)
						return "test-" + str, nil
					},
				},
				opGet{
					Builder:  b,
					Path:     []string{"type"},
					ExpValue: "test-post",
				},
			},
		},
	}

	testops.Run(t, []testops.Env{testops.Env{
		Name: "Builder",
		Func: func(tc testops.TestCase) (func(*testing.T), error) {
			return tc.Runner(nil), nil
		},
	}}, tcs)
}
