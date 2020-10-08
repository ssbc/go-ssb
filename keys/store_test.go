package keys

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/keks/testops"
	"github.com/stretchr/testify/require"
	"modernc.org/kv"

	"go.cryptoscope.co/librarian"
)

type opDo func(t *testing.T, env interface{})

func (op opDo) Do(t *testing.T, env interface{}) {
	op(t, env)
}

func TestStore(t *testing.T) {
	tDir, err := ioutil.TempDir(".", "test-manager-")
	require.NoError(t, err, "mk temp dir")

	t.Cleanup(func() {
		os.RemoveAll(tDir)
	})

	var (
		idx librarian.SeqSetterIndex
		db  *kv.DB
		mgr Store
	)

	tcs := []testops.TestCase{
		{
			Name: "compound test", // TODO: split this into smaller tests
			Ops: []testops.Op{
				opDBCreate{
					Name: filepath.Join(tDir, "testdb"),
					Opts: &kv.Options{},
					DB:   &db,
				},
				opIndexNew{
					DB:    &db,
					Type:  Keys(nil),
					Index: &idx,
				},
				opDo(func(t *testing.T, env interface{}) {
					mgr = Store{idx}
				}),
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeLargeSymmetricGroup,
					Key:    Key("topsecret")},
				opIndexGet{
					Index: &idx,
					Addr: librarian.Addr([]byte{
						30, 0, // type is 30 byte long
						101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						4, 0, // db key is four byte long
						't', 'e', 's', 't', // "test"
					}),

					ExpValue: Keys{Key("topsecret")},
				},
				opDBGet{
					DB: &db,
					Key: []byte{
						30, 0, // type is 30 byte long
						101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						4, 0, // db key is four byte long
						't', 'e', 's', 't', // "test"
					},

					ExpValue: func(ks Keys) []byte {
						var strs = make([]string, len(ks))
						for i := range ks {
							strs[i] = base64.StdEncoding.EncodeToString((ks)[i])
						}

						exp, err := json.Marshal(strs)
						require.NoError(t, err, "json encode of test string")
						return exp
					}(Keys{Key("topsecret")}),
				},
				opStoreAddKey{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeLargeSymmetricGroup,
					Key:    Key("alsosecret"),
				},
				opIndexGet{
					Index: &idx,
					Addr: librarian.Addr([]byte{
						30, 0, // type is 30 byte long
						101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						4, 0, // db key is four byte long
						't', 'e', 's', 't', // "test"
					}),

					ExpValue: Keys{Key("topsecret"), Key("alsosecret")},
				},
				opDBGet{
					DB: &db,
					Key: []byte{
						30, 0, // type is 30 byte long
						101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						4, 0, // db key is four byte long
						't', 'e', 's', 't', // "test"
					},

					ExpValue: func(ks Keys) []byte {
						var strs = make([]string, len(ks))
						for i := range ks {
							strs[i] = base64.StdEncoding.EncodeToString((ks)[i])
						}

						exp, err := json.Marshal(strs)
						require.NoError(t, err, "json encode of test string")
						return exp
					}(Keys{Key("topsecret"), Key("alsosecret")}),
				},
				opStoreSetKey{
					Mgr:    &mgr,
					ID:     ID("foo"),
					Key:    Key("bar"),
					ExpErr: "keys: invalid scheme at (, )",
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeLargeSymmetricGroup,
					ExpRecps: Recipients{
						Recipient{Key: Key("topsecret"), Scheme: SchemeLargeSymmetricGroup},
						Recipient{Key: Key("alsosecret"), Scheme: SchemeLargeSymmetricGroup},
					},
				},
				opStoreRmKeys{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeLargeSymmetricGroup,
				},
				opStoreGetKeys{
					Mgr:    &mgr,
					ID:     ID("test"),
					Scheme: SchemeLargeSymmetricGroup,
					ExpErr: fmt.Sprintf("keys: no such key found at (envelope-large-symmetric-group, %x)", "test"),
				},
			},
		},
		{
			Name: "idxKey encode - short buffer",
			Ops: []testops.Op{
				opDBKeyEncode{
					Key: &idxKey{
						ks: SchemeLargeSymmetricGroup,
						id: ID("test"),
					},
					BufLen: 1,
					ExpErr: "buffer too short: need 38, got 1",
				},
			},
		},
		{
			Name: "idxKey decode",
			Ops: []testops.Op{
				opDBKeyDecode{
					Bytes:  []byte{0}, // only one byte, too short to read type length
					ExpErr: "data too short to read type length",
				},
				opDBKeyDecode{
					Bytes:  []byte{4, 0, 1}, // buffer too short to read type of length 4
					ExpErr: "invalid key - claimed type length exceeds buffer",
				},
				opDBKeyDecode{
					Bytes:  []byte{4, 0, 't', 'e', 's', 't', 6}, // buffer too short to read type of length 4 plus 2 byte size
					ExpErr: "invalid key - claimed type length exceeds buffer",
				},
				opDBKeyDecode{
					Bytes: []byte{
						30, 0, 101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						6, 0, 'f', 'o', 'o', 'b'}, // buffer too short to read id of length 6
					ExpErr: "invalid key - claimed id length exceeds buffer",
				},
				opDBKeyDecode{
					Bytes: []byte{
						30, 0, 101, 110, 118, 101, 108, 111, 112, 101, 45, 108, 97, 114, 103, 101, 45, 115, 121, 109, 109, 101, 116, 114, 105, 99, 45, 103, 114, 111, 117, 112,
						6, 0, 'f', 'o', 'o', 'b', 'a', 'r',
					},
					ExpKey: &idxKey{ks: SchemeLargeSymmetricGroup, id: ID("foobar")},
				},
			},
		},
		{
			Name: "encode idxKey",
			Ops: []testops.Op{
				opDBKeyEncode{
					Key: &idxKey{
						ks: SchemeLargeSymmetricGroup,
						id: ID("test"),
					},
					ExpData: []byte{
						30, 0,
						0x65, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65, 0x2d, 0x6c, 0x61, 0x72, 0x67, 0x65, 0x2d, 0x73, 0x79, 0x6d, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x2d, 0x67, 0x72, 0x6f, 0x75, 0x70,
						4, 0,
						't', 'e', 's', 't',
					},
				},
				opDBKeyEncode{
					Key: &idxKey{
						ks: SchemeDiffieStyleConvertedED25519,
						id: ID("test"),
					},
					ExpData: []byte{
						0x26, 0x0, 0x65, 0x6e, 0x76, 0x65, 0x6c, 0x6f, 0x70, 0x65, 0x2d, 0x69, 0x64, 0x2d, 0x62, 0x61, 0x73, 0x65, 0x64, 0x2d, 0x64, 0x6d, 0x2d, 0x63, 0x6f, 0x6e, 0x76, 0x65, 0x72, 0x74, 0x65, 0x64, 0x2d, 0x65, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39,
						4, 0,
						't', 'e', 's', 't',
					},
				},
			},
		},
	}

	testops.Run(t, []testops.Env{
		{
			Name: "Keys",
			Func: func(tc testops.TestCase) (func(*testing.T), error) {
				return tc.Runner(nil), nil
			},
		},
	}, tcs)
}
