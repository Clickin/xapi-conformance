# Conformance vectors

Each vector is a JSON document validated by `protocol/vector.schema.json`.
Large wire payloads are stored as adjacent `.xml` or `.json` files and referred
to by relative path from the vector manifest.

Vectors must state whether they are required by the protocol or are an
implementation extension. Every vector must include `source.repository`,
`source.commit`, `source.path`, and `source.license`; imported cases also keep
the upstream attribution. Run `go run ./cmd/xapi-validate -vectors vectors`
before the conformance runner.
