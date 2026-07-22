# Conformance vectors

Each vector is a JSON manifest described by `protocol/vector.schema.json`.
That schema covers the manifest and base64 transport envelope, not the decoded
XML/JSON/SSV document. Large wire payloads may be stored as adjacent `.xml`,
`.json`, or `.ssv` files and referred to by relative path from the manifest.
`expect.kind` selects canonical semantic comparison, exact wire-envelope
comparison, or error-class comparison.

Vectors must state whether they are required by the protocol or are an
implementation extension. Every vector must include `source.repository`,
`source.commit`, `source.path`, and `source.license`; imported cases also keep the
upstream attribution.

Run `go run ./cmd/xapi-validate -vectors vectors` to check schema documents,
manifest metadata, and base64 before running the conformance runner. The runner,
not JSON Schema, exercises each profile's wire parser and encoder.
