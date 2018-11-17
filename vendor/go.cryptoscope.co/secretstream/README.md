# secretstream

A port of [secret-handshake](https://github.com/auditdrivencrypto/secret-handshake) to [Go](https://golang.org).

Provides an encrypted bidirectional stream using two [boxstream]s.
Uses [secret-handshake] to negotiate the keys and nonces.

[boxstream]: https://github.com/dominictarr/pull-box-stream
[secret-handshake]: https://github.com/auditdrivencrypto/secret-handshake
