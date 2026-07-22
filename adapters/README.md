# Adapter contract guide

An adapter is any process implementing the HTTP contract in
[`protocol/README.md`](../protocol/README.md). It may use its own DTOs
internally, but only the protocol envelope and canonical JSON are shared.

## Go

Use `net/http`, decode the request envelope, enforce limits before parsing, and
return `ok:false` with a stable error class/path. Run the example with:

```sh
go run ./cmd/xapi-reference
go run ./cmd/xapi-runner -url http://127.0.0.1:8787
```

## Java, JavaScript, and Rust

Java, JavaScript, and Rust adapters should implement the same four endpoints,
advertise only the profiles/options they actually support, and compare the
canonical model rather than serialized bytes. Pin this repository to a commit
or tag in the implementation repository's CI; do not copy protocol DTOs from
an implementation library.

The adapter may be an ordinary HTTP process, or a small JSONL subprocess for
CI. In JSONL mode, read one UTF-8 JSON envelope from stdin, write exactly one
JSON response line to stdout, write logs only to stderr, and exit after EOF.
This keeps Java, Node.js, and Rust adapters independent of HTTP frameworks in
the fast path while preserving the identical envelope and canonical model.

Example request:

```sh
curl -s http://127.0.0.1:8787/decode \
  -H 'content-type: application/json' \
  -d '{"operation":"decode","profile":"nexacro-json-1.0","input":{"encoding":"base64","data":"eyJwYXJhbWV0ZXJzIjpbXSwiZGF0YXNldHMiOltdfQ=="}}'
```

For fixed CI execution, use JSONL subprocess mode. The runner enforces the
process timeout and returns a non-zero exit code if the adapter exits early or
does not produce one response per request:

```sh
go run ./cmd/xapi-runner -command 'go run ./cmd/xapi-reference stdio' -timeout 10s
```

Use `-profile` and `-operation` for deterministic subsets, for example
`-profile nexacro-xml-4000 -operation decode`.
