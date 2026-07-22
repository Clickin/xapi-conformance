# Wire matrix

Every row below maps to one or more vectors. A combination is not silently
omitted: if an adapter cannot support it, the vector is marked optional or the
capability is explicitly absent.

| Area | Combinations |
|---|---|
| Profiles | XPLATFORM XML 4000; Nexacro XML 4000; Nexacro JSON 1.0 |
| Operations | encode; decode; roundtrip for each profile |
| Scalars | STRING; INT/UINT/LONG; FLOAT/DOUBLE; BIGDECIMAL; BOOLEAN; DATE; TIME; DATETIME; BLOB |
| Shape | empty dataset; zero rows; one/multiple rows; multiple datasets; reordered columns; reordered JSON properties |
| Semantics | parameters and parameter type; lexical precision; missing/null/empty; ConstColumn/value; N/I/U/D/O; OrgRow; column ID mapping |
| XML metadata | namespace; `ver`/`version`; parameter text vs attribute; CDATA/entity/control characters |
| Base64 | canonical encoding; rejected whitespace; explicitly ignored whitespace |
| Invalid/security | malformed XML/JSON; duplicate attributes/keys; invalid base64; DTD/external entity; depth, row, column, scalar, blob, and document limits |
| Policy | strict and compatibility mode |

Current required coverage is represented by these vectors:

| Coverage | Vector IDs |
|---|---|
| JSON 1.0 empty decode/encode/roundtrip | `json.empty-dataset.*` |
| XML 4000 decode, row tracking, OrgRow | `xapi-xml.sample-orgrow.decode` |
| XML 4000 CDATA/entity-safe text; XPLATFORM profile | `xapi-xml.sample-cdata.decode` |
| JSON property/row/constant fixture provenance | `xapi-json.sample-basic.decode` |
| Imported upstream XML samples (basic, complex, constants, empty, CDATA, OrgRow, malformed cases) | `xapi-xml.*.imported` |
| xapi-js direct Dataset, parameter attributes, scalar/BLOB/OrgRow fixture | `xapi-js.handler.inline-*` |
| xapi-js inline XML valid/invalid/compatibility cases | `xapi-js.handler.inline-*` |
| xapi-js core and HTTP-adapter XML literals | `xapi-js.*.inline-*` |
| xapi Java XML edge/error/writer literals | `xapi-xml.*.inline-*` |
| xplatform mixed decimal/BLOB/CDATA and Java text-block XML fixtures | `xplatform-xml.test.inline-*` |
| Scalar lexical forms, BLOB, null/missing/empty, constants | `matrix.all-scalars.encode-json` |
| Invalid base64, duplicate JSON keys, malformed JSON | `security.*`, `invalid.malformed-json` |
| DTD/external entity rejection | `security.dtd-external-entity` |
| XML duplicate attributes; malformed XML/JSON | `security.duplicate-xml-attribute`, `invalid.malformed-*` |
| Base64 whitespace policy | `compatibility.base64-whitespace-ignored`, `security.base64-whitespace-rejected` |
| Dataset/row/column/depth/scalar/blob/payload limits | `security.*-limit` |

The required policy dimensions above are represented by executable vectors.
All wire-bearing fixtures found in the pinned relevant tests are imported or
represented with provenance. Upstream tests that assert JavaScript parser
internals, Java type/annotation behavior, framework middleware behavior, or
generated source diagnostics are intentionally tracked as non-wire assertions
in the inventory rather than misrepresented as protocol vectors.
