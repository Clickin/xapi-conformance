# Wire 검증 매트릭스

아래 각 항목은 하나 이상의 실행 가능한 vector와 연결됩니다. Adapter가 특정
조합을 지원하지 않으면 vector가 optional이거나 capability에서 해당 기능을
명시적으로 제외해야 합니다.

“모든 optional mask”는 해당 grammar level에 정의된 optional field의
presence/absence powerset을 뜻합니다. 필수 field 누락은 별도 negative vector로
검증합니다. 생략된 `type`은 `STRING`으로 normalize합니다. 생략된 `size`는
profile별 기본값이 일치하지 않으므로 wire-level 값을 임의로 부여하지 않습니다.

| 영역 | 검증 조합 |
|---|---|
| Profile | XPLATFORM XML 4000; Nexacro XML 4000; Nexacro JSON 1.0; XPLATFORM/Nexacro SSV; XPLATFORM/Nexacro PlatformBinary 5000; PlatformZlib |
| Operation | 모든 profile의 encode/decode/roundtrip |
| Scalar | STRING/CHAR; SHORT/USHORT/INT/UINT/LONG/ULONG; FLOAT/DOUBLE; DECIMAL/BIGDECIMAL; BOOLEAN; DATE/TIME/DATETIME; BLOB/FILE/NULL; 내부 UNDEFINED/DATASET/INVALID tag; type 대소문자 |
| Shape | 빈 document/Dataset; 0/1/복수 row; 복수 Dataset; column 순서 변경; JSON property 순서 변경 |
| Optionality | 모든 root-child mask; XML Parameter/Column/ConstColumn attribute mask; JSON Parameter/Column/ConstColumn property mask; SSV stream/variable/header optional form |
| 의미 | parameter/type; lexical 정밀도; missing/null/empty; ConstColumn/value; N/I/U/D/O; 기본 N 생략; OrgRow; column ID mapping |
| XML metadata | profile별 namespace/`ver`; declaration 위치; direct Dataset; optional Parameters/ColumnInfo/Rows; parameter text/attribute; ConstColumn 순서; CDATA/entity/control character |
| JSON metadata | 필수 version; optional Parameters/Datasets; Dataset/ColumnInfo/Column/Rows 필드; 기본 type; numeric lexical; 생략된 `_RowType_`; O row 인접성; entity-like literal text |
| SSV framing | stream header/code page; variable/Dataset; RS/US; type/length/value form; packed variable; `_Const_`; `_RowType_`; state marker; null record; Latin-1/Windows-1252 |
| BLOB metadata | XML `enc="base64"`; 대소문자; compatibility `encrypt="base64"`; canonical base64 |
| Transport | canonical base64; whitespace 거부/허용; 모든 profile의 `FF AD` + zlib |
| 오류/보안 | 필수 field 누락; 지원하지 않는 type; malformed payload; duplicate attribute/key; invalid base64; DTD/external entity; depth/row/column/scalar/blob/payload limit |
| 정책 | strict/compatibility; decode 허용 범위와 canonical encode; root/Dataset `saveType` |

## 실행 vector 연결

| 검증 범위 | Vector ID 또는 test |
|---|---|
| JSON 1.0 empty decode/encode/roundtrip | `json.empty-dataset.*` |
| XML 4000 row state와 OrgRow | `xapi-xml.sample-orgrow.decode` |
| XML CDATA/entity-safe text | `xapi-xml.sample-cdata.decode` |
| JSON property/row/constant sample | `xapi-json.sample-basic.decode` |
| 기본/복합/constant/empty/CDATA/OrgRow/malformed XML sample | `xapi-xml.*.imported` |
| Direct Dataset, parameter attribute, scalar/BLOB/OrgRow, HTTP payload fixture | `*.inline-*` |
| XML entity와 profile framing | `*.format-entities.decode`, `*.entity.encode` |
| Compatibility XML namespace/version | `*.commercial-framing.*` |
| JSON 기본값, summary column, O-row 인접성 | `nexacro-json.format-defaults.decode` |
| JSON entity-like literal과 기본 N row | `nexacro-json.entity-literals.decode` |
| Nexacro/XPLATFORM SSV | `nexacro-ssv.*`, `xplatform-ssv.*` |
| XML optional mask | aggregate `*-xml.optional-combinations.decode`; 독립 `isolated.*-xml-*`; root `*-xml.optional-root-*` |
| JSON optional mask | aggregate `nexacro-json.optional-combinations.decode`; 독립 `isolated.json-*`; root `nexacro-json.optional-root-*` |
| SSV optional mask | aggregate `*-ssv.optional-combinations.decode`; 독립 `isolated.*-ssv-*`; root `*-ssv.optional-root-*` |
| 필수 ID/container/header 누락과 unknown type | `*.missing-*.strict`, `*.unknown-type.strict` |
| XML BLOB `enc`와 `encrypt` alias | `*.blob-column.decode`, `*.blob-const.decode`, `*.blob-encoding.encode`, `*.blob-encrypt-alias.decode` |
| XML declaration/namespace/layout/order 위반 | `*-xml.*.strict` |
| Scalar lexical, BLOB, state, constant | `matrix.all-scalars.encode-json` |
| Grouped/hex integer, Boolean alias, special float, decimal, invalid default | `compatibility.source-scalar-lexicals.encode`, `compatibility.json-floating-special-values.encode` |
| FILE remapping과 JSON BLOB 검증 | `json.file.encode`, `nexacro-json.blob.decode`, `security.invalid-json-blob` |
| Invalid base64, duplicate JSON key, malformed JSON | `security.*`, `invalid.malformed-json` |
| DTD/external entity | `security.dtd-external-entity` |
| Duplicate XML attribute | `security.duplicate-xml-attribute` |
| Base64 whitespace | `compatibility.base64-whitespace-ignored`, `security.base64-whitespace-rejected` |
| Dataset/row/column/depth/scalar/blob/payload limit | `security.*-limit` |
| Profile별 PlatformZlib | `zlib.*` |
| PlatformBinary 5000 framing/type/row/OrgRow/limit | `*.binary.*`, `internal/codec/binary_test.go` |
| `saveType` 1–5, Dataset override, original/removed row | `policy.save-type-changed.encode`, saveType unit tests |
| Encode/roundtrip root powerset | `*-optional-root-*-encode`, `*-optional-root-*-roundtrip` |

필수 정책 차원은 모두 실행 vector로 표현합니다. 가져온 fixture는 provenance를
유지합니다. Parser 내부 동작, framework middleware, 생성 진단처럼 wire를
통과하지 않는 assertion은 protocol vector로 가장하지 않고 inventory에만
기록합니다.
