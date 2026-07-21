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

## Sources

Fixtures and assertions are imported from the Clickin `xapi-js`,
`xplatform-xml`, and `xapi` repositories. Each imported source keeps its
original attribution and license metadata.

## Scope

The vector corpus is intended to cover the complete supported wire matrix:
profiles, scalar types, lexical forms, null/missing/empty values, parameters,
datasets, constants, row states, original rows, ordering variants, BLOBs,
malformed input, limits, and compatibility policies.
