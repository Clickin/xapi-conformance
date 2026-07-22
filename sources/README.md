# Source inventory

The three directories are shallow clones used for analysis and provenance.
They are not adapter dependencies and their implementation types do not cross
the HTTP protocol boundary.

| Repository | Pinned commit | Relevant test areas |
|---|---|---|
| [Clickin/xapi-js](https://github.com/Clickin/xapi-js) | `244339098838d098d7087588d039a24d26448b5e` | `packages/core/test/{codec,handler,schema,xapi-data}.test.ts` |
| [Clickin/xplatform-xml](https://github.com/Clickin/xplatform-xml) | `cda8b6b31f64511ff9d22f64539c4b78e862455d` | `xplatform-xml-micronaut/src/test/.../XplatformXmlSerdeTest.java`, processor tests |
| [Clickin/xapi](https://github.com/Clickin/xapi) | `e0d54759162bbb06aef8a1bc4c557f9fe7336991` | `xapi-json/src/test`, `xapi-xml/src/test/resources/xapi` |

Inventory was collected on 2026-07-21. Imported vectors must point to an exact
commit and source path in their `source` object.

Machine-readable counts and the current conversion status are in
[`inventory.json`](inventory.json).

The reproducible importer in `cmd/xapi-import` converts pinned XML fixtures
and XML literals found in the relevant JavaScript/Java tests into vectors while
retaining commit/path/license provenance. Malformed source fixtures become
negative vectors under `vectors/invalid/imported`. CI runs it after checking
out the pinned source repositories.

Non-wire assertions (for example type-level schema tests, framework adapter
mock tests, and annotation-processor diagnostics) are inventoried but are not
pretended to be Dataset wire vectors. Their wire-bearing literals are imported
when they can be isolated deterministically.
