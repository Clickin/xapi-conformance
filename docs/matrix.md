# Wire matrix

Every row below maps to one or more vectors. A combination is not silently
omitted: if an adapter cannot support it, the vector is marked optional or the
capability is explicitly absent.

“Every optional mask” means the Cartesian product of presence/absence for the
documented optional fields at that grammar level. Required-field omission is
covered separately by negative vectors. Omitted `type` normalizes to `STRING`;
omitted `size` remains omitted because the published and commercial defaults
are not consistent enough to assign one wire-level value.

| Area | Combinations |
|---|---|
| Profiles | XPLATFORM XML 4000; Nexacro XML 4000; Nexacro JSON 1.0; XPLATFORM SSV; Nexacro SSV |
| Operations | encode; decode; roundtrip for each profile |
| Scalars | STRING/CHAR; SHORT/USHORT/INT/UINT/LONG/ULONG; FLOAT/DOUBLE; DECIMAL/BIGDECIMAL; BOOLEAN; DATE; TIME; DATETIME; BLOB; FILE; NULL; case-insensitive type names |
| Shape | empty document/dataset; zero rows; one/multiple rows; multiple datasets; reordered columns; reordered JSON properties |
| Optionality | every root-child presence mask; every XML Parameter/Column/ConstColumn attribute-presence mask; every JSON Parameter/Column/ConstColumn property-presence mask; every SSV stream/variable/header optional-form mask |
| Semantics | parameters and parameter type; lexical precision; missing/null/empty; ConstColumn/value; N/I/U/D/O; omitted default N; optional OrgRow; column ID mapping |
| XML metadata | profile-specific namespace and `ver`; declaration placement; direct Dataset layout; optional Parameters/ColumnInfo/Rows; parameter text vs attribute; ConstColumn order; CDATA/entity/control characters |
| JSON metadata | required version; optional Parameters/Datasets; required Dataset/ColumnInfo/Column/Rows fields; default parameter/column types; numeric lexical values; omitted `_RowType_` as N; O row adjacency; XML entity-like text remains literal |
| SSV framing | stream header/optional code page; optional variables/datasets; configurable RS/US; optional type/length/value forms; `_Const_`; `_RowType_`; Nexacro ETX vs XPLATFORM STX states; null record; no XML entity processing |
| BLOB metadata | XML `enc="base64"` requirement in strict mode; case-insensitive value; commercial `encrypt="base64"` decode alias; canonical base64 value |
| Base64 transport | canonical encoding; rejected whitespace; explicitly ignored whitespace |
| Invalid/security | missing required fields; unsupported types; malformed XML/JSON/SSV; duplicate attributes/keys; invalid base64; DTD/external entity; depth, row, column, scalar, blob, and document limits |
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
| Published XML entity decoding and profile framing | `nexacro-xml.format-entities.decode`, `xplatform-xml.format-entities.decode`, `*.entity.encode` |
| Commercial XML framing: XPLATFORM strict rejection/compatibility acceptance; Nexacro omitted `ver` acceptance | `xplatform-xml.commercial-framing.*`, `nexacro-xml.commercial-framing.decode` |
| Published JSON defaults, summary columns, and O-row adjacency | `nexacro-json.format-defaults.decode` |
| JSON entity-like literals and default N row | `nexacro-json.entity-literals.decode` |
| Nexacro and XPLATFORM SSV parsing/encoding/state markers | `nexacro-ssv.*`, `xplatform-ssv.*` |
| Exhaustive XML optional masks and root-child masks, both profiles | `nexacro-xml.optional-combinations.decode`, `xplatform-xml.optional-combinations.decode`, `*-xml.optional-root-*.decode` |
| Exhaustive JSON optional masks and root-child masks | `nexacro-json.optional-combinations.decode`, `nexacro-json.optional-root-*.decode` |
| Exhaustive SSV optional masks and root/item forms, both profiles | `nexacro-ssv.optional-combinations.decode`, `xplatform-ssv.optional-combinations.decode`, `*-ssv.optional-root-*.decode` |
| Missing required IDs/containers/headers and unsupported types | `*.missing-*.strict`, `*.unknown-type.strict` |
| XML BLOB `enc` contract and commercial `encrypt` alias | `*.blob-column.decode`, `*.blob-const.decode`, `*.blob-encoding.encode`, `*.blob-encrypt-alias.decode` |
| XML declaration, namespace, direct Dataset, and ConstColumn-order violations | `*-xml.*.strict` |
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
