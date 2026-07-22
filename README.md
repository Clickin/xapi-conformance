# xapi-conformance

Language-neutral conformance vectors and HTTP contracts for XPLATFORM/Nexacro
Dataset XML and JSON implementations.

Implementations expose a small HTTP service. The runner sends wire payloads and
compares the result with a canonical, language-neutral representation. Encoded
XML/JSON is never compared byte-for-byte; it is decoded and normalized first.

## Contract

Required endpoints:

- `GET /capabilities`
- `POST /decode`
- `POST /encode`
- `POST /roundtrip`

The wire contract is described in [`protocol/README.md`](protocol/README.md).

## Run the reference conformance suite

The CI path validates the committed vector corpus and runs the reference
adapter. Source inventory checkout is only needed when regenerating imported
vectors and may require access to private upstream repositories.

```sh
go test ./...
go run ./cmd/xapi-validate -vectors vectors
go run ./cmd/xapi-runner -command 'go run ./cmd/xapi-reference stdio' \
  -json results.json -junit results.xml
```

### Run through npm or pnpm

The published wrapper downloads and runs the matching Go runner module. Go
1.22+ is required.

```sh
pnpm dlx @clickin/xapi-conformance -url http://127.0.0.1:8787
npx @clickin/xapi-conformance -url http://127.0.0.1:8787
```

The wrapper accepts the runner flags documented below. Use `-vectors` to
override the packaged vector directory.

HTTP compatibility mode is also available:

```sh
go run ./cmd/xapi-reference
go run ./cmd/xapi-runner -url http://127.0.0.1:8787
```

The runner supports `-profile`, `-operation`, `-parallel`, `-timeout`, and
`-filter` selectors. It compares canonical semantic JSON, not wire bytes.

## Sources

Fixtures and assertions are imported from the Clickin `xapi-js`,
`xplatform-xml`, and `xapi` repositories. Each imported source keeps its
original attribution and license metadata.

## Scope

The vector corpus is intended to cover the complete supported wire matrix:
profiles, scalar types, lexical forms, null/missing/empty values, parameters,
datasets, constants, row states, original rows, ordering variants, BLOBs,
malformed input, limits, and compatibility policies.
