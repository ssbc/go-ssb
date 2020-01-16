# Test vectors

The following test vectors are provided to check your implementation of this spec.
If you're going to test any of them make sure you pass:
- `box1.json`
- `unbox1.json`

The other test vectors provided are here to help you check particular steps
which are crucial the box / unbox process.


## Format

All test vectors follow the format:

```
{
  type: String,        // machine readable
  description: String, // human readable
  input: {
    ...                // base64 encoded properties
  },
  output: {
    ...                // base64 encoded properties
  }
}
```

where `input` and `output` format will depend on the `type`.
