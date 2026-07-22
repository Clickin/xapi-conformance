# XAPI Dataset conformance protocol

Protocol version: `1.0`.

The protocol is deliberately independent of Java, JavaScript, Go, Rust, or
any implementation DTO. A conformance adapter is an HTTP process that accepts
JSON envelopes and transports the original XML/JSON wire document as base64.
For fixed CI runs, the same envelope is also supported as JSON Lines over
stdin/stdout: invoke the reference adapter as `go run ./cmd/xapi-reference
stdio`. One request line produces one response line; logs go to stderr.

All requests and responses use `application/json`. The `input.data` and
`output.data` fields contain base64-encoded wire bytes so XML and JSON payloads
are transported identically.

## Endpoints

`GET /capabilities` returns the adapter contract. `POST /decode` converts wire
bytes to canonical JSON, `POST /encode` converts canonical JSON to wire bytes,
and `POST /roundtrip` decodes then encodes and returns both canonical value and
the resulting bytes.

## Request

```json
{
  "case": "xml.dataset.basic",
  "operation": "decode",
  "profile": "nexacro-xml-4000",
  "input": {"encoding": "base64", "data": "..."},
  "options": {"strict": true}
}
```

`operation` is one of `decode`, `encode`, or `roundtrip`. The endpoint must
reject an operation/profile mismatch with `unsupported-operation` or
`unsupported-profile`, never silently select a different profile.

`input` is required for `decode` and `roundtrip`; `value` is required for
`encode`. `options` is an object and may include `strict`, `base64Whitespace`,
and `limits` (`payloadBytes`, `datasets`, `rows`, `columns`, `scalarBytes`, and
`blobBytes`). Unknown options are an `invalid-request` error in strict mode.
The default maximum request body is 10 MiB and the default operation timeout
is 10 seconds. Adapters must return a response before the timeout;
the runner terminates an unresponsive process and reports `timeout`.

## Success

```json
{
  "ok": true,
  "value": {
    "parameters": [],
    "datasets": []
  }
}
```

`value` is the canonical representation. Values are lexical strings unless a
field is explicitly structural. BLOB values are base64 strings.

For `encode` and `roundtrip`, a successful response also contains
`output: {"encoding":"base64","data":"..."}`. Base64 is RFC 4648 without
line breaks by default. Whitespace is accepted only when
`options.base64Whitespace` is `ignore`; it is rejected by default.

## Failure

```json
{
  "ok": false,
  "error": {
    "class": "malformed-input",
    "path": "Root.Dataset.Rows.Row.Col",
    "message": "..."
  }
}
```

Common error classes are `invalid-request`, `unsupported-operation`,
`unsupported-profile`, `malformed-input`, `invalid-value`, `limit-exceeded`,
`timeout`, and `internal`. Conformance compares `error.class` and, where
specified by a vector, `path`.
Human-readable `message` text is diagnostic and is not portable.

## Capabilities

`GET /capabilities` returns supported profiles, operations, options, limits, and
protocol version, for example:

```json
{"protocolVersion":"1.0","implementation":"reference-go","profiles":[
 {"name":"nexacro-json-1.0","operations":["decode","encode","roundtrip"],
  "options":["strict","base64Whitespace","limits"],"limits":{"payloadBytes":10485760}}
]}
```

The runner skips unsupported optional profiles but fails when a required
capability is missing. A server exits with a non-zero status for startup
configuration errors; it must not restart itself after a protocol failure.

## Canonical value

The canonical model preserves wire semantics rather than language types:

```json
{
  "parameters": [{"id":"ErrorCode","type":"INT","lexical":"0","state":"value"}],
  "datasets": [{
    "id":"output",
    "columns":[{"id":"stockCode","type":"STRING","index":0}],
    "constColumns":[], "rows":[{"type":"N","orgRow":null,
      "values":{"stockCode":{"state":"value","lexical":"10001"}}}]
  }]
}
```

Parameters and columns retain wire order through `index`; object key ordering
is otherwise ignored. Dataset column lookup is by `id`, so reordered columns
must compare semantically equal. `state` is one of `value`, `missing`,
`null`, or `empty`; `lexical` is always a string when `state` is `value` or
`empty`. This preserves DATE/TIME/DATETIME precision, BIGDECIMAL spelling,
and all scalar values without imposing a host-language type. BLOB lexical
values are base64 and `orgRow` is a complete row snapshot using the same cell
model. XML namespace and version metadata, parameter text/attribute form, and
JSON ordering are represented under `wire` when they are semantically relevant.

See [`canonical.schema.json`](canonical.schema.json) and
[`vector.schema.json`](vector.schema.json) for machine-readable definitions.
Requests, responses, and capabilities are defined by
[`request.schema.json`](request.schema.json),
[`response.schema.json`](response.schema.json), and
[`capabilities.schema.json`](capabilities.schema.json).

## Security and limits

Adapters must not resolve DTDs or external entities, must reject duplicate JSON
keys and duplicate XML attributes, and must enforce payload, nesting, dataset,
row, column, scalar, and blob limits before allocating unbounded memory.
Malformed XML/JSON and invalid base64 are protocol errors, not process crashes.
