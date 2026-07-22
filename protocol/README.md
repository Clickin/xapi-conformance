# XAPI Dataset conformance protocol

Protocol version: `1.0`.

The protocol is deliberately independent of Java, JavaScript, Go, Rust, or
any implementation DTO. A conformance adapter is an HTTP process that accepts
JSON envelopes and transports the original XML, JSON, or SSV wire document as
base64. For fixed CI runs, the same envelope is also supported as JSON Lines
over stdin/stdout: invoke the reference adapter as `go run
./cmd/xapi-reference stdio`. One request line produces one response line; logs
go to stderr.

All requests and responses use `application/json`. The `input.data` and
`output.data` fields contain base64-encoded wire bytes so all profiles are
transported identically.

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
`ssvUnitSeparator`, `ssvRecordSeparator`, and `limits` (`payloadBytes`,
`datasets`, `rows`, `columns`, `scalarBytes`, and `blobBytes`). SSV separators
must each be one Unicode scalar. Unknown options are an `invalid-request` error
in strict mode. The default maximum request body is 10 MiB and the default
operation timeout is 10 seconds. Adapters must return a response before the
timeout; the runner terminates an unresponsive process and reports `timeout`.

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
`output: {"encoding":"base64","data":"..."}`. Base64 is RFC 4648 without line
breaks by default. Input whitespace is accepted only when
`options.base64Whitespace` is `true`. Vectors with `expect.kind` equal to
`wire` compare this output envelope exactly.

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
model. XML namespace/version and parameter form, JSON version, and SSV framing
metadata are represented under `wire` when they are semantically relevant.

See [`canonical.schema.json`](canonical.schema.json) and
[`vector.schema.json`](vector.schema.json) for machine-readable definitions.
Requests, responses, and capabilities are defined by
[`request.schema.json`](request.schema.json),
[`response.schema.json`](response.schema.json), and
[`capabilities.schema.json`](capabilities.schema.json).

## Schema and wire validation

The files under `protocol/*.schema.json` are JSON Schema documents because the
request envelope, response envelope, vector manifest, and canonical value are
JSON for every profile. They validate those JSON structures. The
base64-encoded `input.data` and `output.data` fields are intentionally opaque
to JSON Schema: the schemas do not decode or validate embedded XML, JSON
Dataset payloads, or SSV records.

Wire-format conformance is executable. The runner sends each vector to the
adapter under the selected profile and checks the canonical value, exact wire
output, or expected error. Strict XML vectors cover XML declarations,
namespaces, element/attribute grammar, entities, and row structure. Strict JSON
vectors cover Dataset JSON fields and types. Strict SSV vectors cover stream
headers, RS/US framing, headers, rows, state markers, and terminal null records.

Entity handling is profile-specific:

- XML decodes XML character/entity references. XML encoding represents line
  feed as `&#10;` where required by the XAPI wire behavior.
- JSON applies only JSON string escaping. Text such as `&amp;` or `&#10;`
  remains literal text and is never passed through an XML entity codec.
- SSV does not apply XML entity processing; separator and state-marker rules
  define its framing.

For default rows, XML may omit `Row@type` and JSON may omit `_RowType_`; both
decode as `N`. XML omits `OrgRow` and JSON omits the adjacent `O` row when no
original row exists.

## Security and limits

Adapters must not resolve DTDs or external entities, must reject duplicate JSON
keys and duplicate XML attributes, and must enforce payload, nesting, dataset,
row, column, scalar, and blob limits before allocating unbounded memory.
Malformed XML/JSON/SSV and invalid base64 are protocol errors, not process crashes.
