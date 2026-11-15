# Conformance 검증 보고서

검증일: 2026-07-23  
최종 갱신: 2026-11-15

## 결론

지원하는 모든 wire 계약은 이름이 있는 profile, 실행 가능한 codec path, 필수
conformance vector로 연결되어 있습니다. 범위에는 Platform XML, JSON, SSV,
PlatformBinary, PlatformZlib이 포함됩니다.

하나의 “fullspec” XML/JSON/SSV 문서로 조합 검증을 대신하지 않습니다. 대형
`optional-combinations` 문서는 회귀 fixture로 유지하지만, 192개의 독립
`isolated.*` vector가 Parameter, Column, ConstColumn, Dataset child, SSV item
form, type 대소문자 mask를 각각 별도 문서에서 실행합니다. Root mask도 decode,
encode, roundtrip별로 분리되어 있습니다.

## 실행 결과

vector 586개를 유지합니다.

- decode 489, encode 54, roundtrip 43
- canonical expectation 412, error expectation 79, exact-wire expectation 95
- suite: vendor 584, security 2
- 모든 vector는 필수 capability 대상입니다.
- `go test ./...`: 4 package 통과, 2 package는 test 없음
- `go run ./cmd/xapi-validate -vectors vectors`: 586 vector 검증

Profile별 operation 수:

| Profile | Decode | Encode | Roundtrip |
|---|---:|---:|---:|
| `nexacro-json-1.0` | 55 | 19 | 6 |
| `nexacro-xml-4000` | 195 | 9 | 7 |
| `xplatform-xml-5000` | 101 | 9 | 6 |
| `nexacro-ssv` | 68 | 6 | 10 |
| `xplatform-ssv` | 65 | 6 | 10 |
| `nexacro-binary-5000` | 3 | 3 | 2 |
| `xplatform-binary-5000` | 2 | 2 | 2 |

## Profile 범위

| Wire 계약 | Conformance profile |
|---|---|
| Nexacro Dataset JSON 1.0 | `nexacro-json-1.0` |
| Nexacro Platform XML 4000 | `nexacro-xml-4000` |
| XPLATFORM XML 5000 | `xplatform-xml-5000` |
| Nexacro SSV | `nexacro-ssv` |
| XPLATFORM SSV | `xplatform-ssv` |
| Nexacro PlatformBinary 5000 | `nexacro-binary-5000` |
| XPLATFORM PlatformBinary 5000 | `xplatform-binary-5000` |
| `FF AD` + zlib transport | 모든 profile의 `options.zlib` |

## 호환성 검증

### Row와 saveType

- JSON/SSV의 `O` row는 바로 앞 materialized row의 `orgRow`가 됩니다.
- Root와 Dataset에 `saveType`을 표현하며 Dataset 값이 우선합니다. 둘 다 없으면
  `2`(normal)입니다.
- encode는 `saveType`을 거부하지 않고 적용합니다. 이전의
  `policy.save-type-changed.encode`(encode 거부)는 원본 driver 동작과 달라
  삭제했습니다.
- 행 선택, `D` 행 위치, 원본 행 처리는 `vectors/driver/savetype`과
  `vectors/driver/const`의 `update-row-unchanged-values-*`가 검증합니다
  (`protocol/README.md` 참조).

### Scalar와 BLOB

- BOOLEAN→`int`, LONG/ULONG/DECIMAL/BIGDECIMAL→`bigdecimal`, DOUBLE→`float`,
  FILE→`blob` wire type 변환을 검증합니다.
- Grouped/hex integer, Boolean alias, special float, decimal 표기,
  invalid-to-default를 검증합니다.
- BLOB/FILE은 canonical Base64를 사용하며 decode와 encode 모두 검증합니다.
- UNDEFINED, NULL, DATASET, INVALID 내부 tag는 PlatformBinary exact-wire
  vector로 검증합니다.
- TIME과 DATETIME의 9자리/17자리 wire form을 검증합니다.

### Format과 framing

- XML은 canonical 및 compatibility namespace/version을 처리하며 encode 결과는
  안정적입니다. DTD, external entity, duplicate attribute, invalid BLOB은
  거부합니다.
- JSON은 duplicate key, numeric value, property 순서, optional container,
  summary column, O-row 인접성, entity-like text를 검증합니다.
- SSV는 두 state-marker dialect, packed variable, custom separator, null record,
  `_Const_`, `_RowType_`, Latin-1/Windows-1252를 검증합니다.
- PlatformBinary는 block length/count를 제한하고 truncated 또는 oversized
  payload를 거부합니다.

## Canonical 정책

- `value`, `missing`, `null`, `empty` 네 wire state를 구분합니다.
- Strict mode는 정규화된 grammar를 강제합니다.
- Compatibility mode(`strict:false`)는 문서화된 empty-container 생략을
  허용합니다.
- Decode한 lexical 표기를 보존하고 scalar 변환은 encode 경계에서 적용합니다.
- XML word row type은 대소문자를 구분하며 알 수 없는 값은 기본 N으로
  normalize합니다.

## 라이선스 적용 원본 드라이버 재검증 (2026-11-15)

Nexacro X-API 2.0.7과 XPLATFORM X-API Jakarta 1.0에 demo license를 적용해
다시 실행했습니다. 두 license 모두 `isValidTime()=true`로 검증됩니다.
license 검사를 거치는 `PlatformRequest.receiveData()`/`PlatformResponse.sendData()`
경로의 출력은 `DataSerializerFactory` serializer 출력과 byte 단위로 같습니다.

| Profile | 원본 드라이버 (`-suite vendor`) |
|---|---:|
| `nexacro-json-1.0` | 78/78 |
| `nexacro-xml-4000` | 211/211 |
| `xplatform-xml-5000` | 116/116 |
| `nexacro-ssv` | 84/84 |
| `xplatform-ssv` | 81/81 |
| `nexacro-binary-5000` | 8/8 |
| `xplatform-binary-5000` | 6/6 |

판단 기준은 원본 드라이버의 실제 출력입니다. XPLATFORM XML encode는 드라이버와
같이 `<Root xmlns="http://www.tobesoft.com/platform/dataset" ver="5000">`을
씁니다. 공식 Dataset XML Format 문서의 예제는 `.../platform/Dataset`과
`ver="4000"`이지만 XPLATFORM client는 이 xmlns를 무시합니다. XML, Binary, SSV,
JSON과 압축 전송은 XPLATFORM 9.2/Nexacro client help의 `transaction()`
`nDataType`, `bCompress`에 명시된 동작입니다.

## 미해결 wire gap

지원 protocol matrix에는 없습니다.
