All test vectors follow the format:

```
{
  type: String,        // machine readable
  description: String, // human readable
  input: { ... },
  output: { ... }
}
```

where `input` and `output` format will depend on the `type`.

All keys in the vector are base64 encoded.

