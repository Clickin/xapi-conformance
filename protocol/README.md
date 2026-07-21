# HTTP conformance protocol

All requests and responses use `application/json`. The `input.data` and
`output.data` fields contain base64-encoded wire bytes so XML and JSON payloads
are transported identically.

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

`operation` is one of `decode`, `encode`, or `roundtrip`.

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

Conformance compares `error.class` and, where specified by a vector, `path`.
Human-readable `message` text is diagnostic and is not portable.

## Capabilities

`GET /capabilities` returns supported profiles, operations, options, limits, and
protocol version. The runner uses it to skip unsupported optional profiles but
fails when a required capability is missing.

## Canonical value

The canonical model preserves wire semantics rather than language types:

```json
{
  "parameters": [{"id":"ErrorCode","type":"INT","value":"0"}],
  "datasets": [{
    "id":"output",
    "columns":[{"id":"stockCode","type":"STRING"}],
    "constColumns":[],
    "rows":[{"type":"N","values":{"stockCode":"10001"}}]
  }]
}
```

Column and object ordering is ignored unless a vector explicitly marks order as
significant. Missing, null, and empty values remain distinct.
