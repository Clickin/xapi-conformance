# XAPI Dataset conformance protocol

Protocol version: `1.0`

이 protocol은 Java, JavaScript, Go, Rust 또는 특정 구현체 DTO와 독립적입니다.
Adapter는 JSON envelope를 받고 선택한 profile의 wire bytes를 base64로
전송합니다. XML, JSON, SSV, PlatformBinary를 동일한 envelope로 처리합니다.

HTTP 외에 JSON Lines 기반 stdin/stdout mode도 지원합니다.

```sh
go run ./cmd/xapi-reference stdio
```

Runner는 case 전 `{"operation":"capabilities"}`를 한 줄 전송합니다. Adapter는
HTTP `GET /capabilities`와 같은 capability 객체를 한 줄로 응답해야 합니다.
이후 요청 한 줄마다 응답 한 줄을 stdout에 출력하며 log는 stderr에만 기록합니다.
모든 HTTP 요청과 응답의 Content-Type은 `application/json`입니다.
`input.data`와 `output.data`는 base64로 인코딩한 wire bytes입니다.

## Endpoint

- `GET /capabilities`: adapter가 지원하는 계약 조회
- `POST /decode`: wire bytes를 canonical JSON으로 변환
- `POST /encode`: canonical JSON을 wire bytes로 변환
- `POST /roundtrip`: decode 후 다시 encode하고 canonical 값과 출력 bytes 반환

## 요청

```json
{
  "case": "xml.dataset.basic",
  "operation": "decode",
  "profile": "nexacro-xml-4000",
  "input": {"encoding": "base64", "data": "..."},
  "options": {"strict": true}
}
```

`operation`은 `decode`, `encode`, `roundtrip` 중 하나입니다. Endpoint와
operation 또는 profile이 일치하지 않으면 `unsupported-operation` 또는
`unsupported-profile`을 반환해야 하며 다른 profile을 자동 선택하면 안 됩니다.

- `decode`, `roundtrip`: `input` 필수
- `encode`: `value` 필수
- `options.strict`: 정규화된 필수 grammar 검증
- `options.base64Whitespace`: base64 입력 공백 허용
- `options.zlib`: encode/roundtrip 출력에 PlatformZlib 적용
- `options.ssvUnitSeparator`: SSV unit separator 지정
- `options.ssvRecordSeparator`: SSV record separator 지정
- `options.limits`: `payloadBytes`, `datasets`, `rows`, `columns`,
  `scalarBytes`, `blobBytes` 제한

SSV separator는 각각 Unicode scalar 하나여야 합니다. Strict mode에서 알 수
없는 option은 `invalid-request`입니다. 기본 최대 HTTP request body는 10 MiB,
기본 operation timeout은 10초입니다. Timeout 안에 응답하지 않으면 runner가
요청 또는 subprocess를 종료하고 `timeout`으로 기록합니다.

## 성공 응답

```json
{
  "ok": true,
  "value": {
    "parameters": [],
    "datasets": []
  }
}
```

`value`는 canonical 표현입니다. 구조 필드를 제외한 값은 lexical string으로
보존하며 BLOB lexical 값은 base64 string입니다.

`encode`와 `roundtrip` 성공 응답에는 다음 output도 포함합니다.

```json
{"encoding":"base64","data":"..."}
```

기본 base64 출력에는 줄바꿈이 없습니다. 입력의 base64 공백은
`options.base64Whitespace`가 `true`일 때만 허용합니다. `expect.kind`가
`wire`인 벡터는 output envelope를 정확히 비교합니다.

## 실패 응답

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

공통 error class:

- `invalid-request`
- `unsupported-operation`
- `unsupported-profile`
- `malformed-input`
- `invalid-value`
- `limit-exceeded`
- `timeout`
- `internal`

Conformance 비교는 `error.class`와 벡터에 명시된 `path`를 사용합니다.
사람이 읽는 `message`는 진단용이므로 구현체 간 동일성을 요구하지 않습니다.

## Capabilities

`GET /capabilities`는 protocol version, 구현체 이름, profile별 operation,
option, limit을 반환합니다.

```json
{
  "protocolVersion": "1.0",
  "implementation": "reference-go",
  "profiles": [{
    "name": "nexacro-json-1.0",
    "operations": ["decode", "encode", "roundtrip"],
    "options": ["strict", "base64Whitespace", "limits", "zlib"],
    "limits": {"payloadBytes": 10485760}
  }]
}
```

Runner는 optional profile이 지원되지 않으면 건너뛸 수 있지만 required
capability가 없으면 실패해야 합니다. 시작 설정 오류는 process의 non-zero exit로
보고하고 개별 protocol 오류 때문에 server를 재시작하면 안 됩니다.

`zlib`은 모든 profile과 조합 가능한 transport option입니다. Decode는 `FF AD`로
시작하는 payload를 자동 감지합니다. Encode와 roundtrip에서 `options.zlib`이
`true`이면 profile bytes를 `FF AD`와 표준 zlib stream으로 감쌉니다.

## Profile

| Profile | Wire 형식 | Operation |
|---|---|---|
| `nexacro-json-1.0` | Nexacro Dataset JSON 1.0 | decode, encode, roundtrip |
| `nexacro-xml-4000` | Nexacro Platform XML 4000 | decode, encode, roundtrip |
| `xplatform-xml-4000` | XPLATFORM XML 4000 | decode, encode, roundtrip |
| `nexacro-ssv` | Nexacro SSV | decode, encode, roundtrip |
| `xplatform-ssv` | XPLATFORM SSV | decode, encode, roundtrip |
| `nexacro-binary-5000` | Nexacro PlatformBinary 5000 | decode, encode, roundtrip |
| `xplatform-binary-5000` | XPLATFORM PlatformBinary 5000 | decode, encode, roundtrip |

## Canonical value

Canonical model은 host language type이 아니라 wire 의미를 보존합니다.

```json
{
  "parameters": [
    {"id":"ErrorCode","type":"INT","lexical":"0","state":"value"}
  ],
  "datasets": [{
    "id":"output",
    "columns":[{"id":"stockCode","type":"STRING","index":0}],
    "constColumns":[],
    "rows":[{
      "type":"N",
      "orgRow":null,
      "values":{"stockCode":{"state":"value","lexical":"10001"}}
    }]
  }]
}
```

Parameter와 column의 wire 순서는 `index`로 보존합니다. JSON object key 순서는
비교하지 않습니다. Dataset column은 `id`로 조회하므로 column 순서가 바뀐
payload도 의미가 같으면 동일하게 비교합니다.

Cell `state`:

- `value`: 값이 있으며 `lexical` 필수
- `missing`: 필드 또는 cell이 wire에 없음
- `null`: 명시적인 null
- `empty`: 빈 값이며 `lexical`은 빈 string

이 모델은 DATE/TIME/DATETIME 정밀도, BIGDECIMAL 표기, scalar lexical 값을
host type 변환 없이 유지합니다. `orgRow`는 같은 cell model을 사용하는 완전한
원본 row snapshot입니다. Profile별 namespace/version, parameter 형식, JSON
version, SSV framing 정보는 의미가 있을 때 `wire` 아래에 보존합니다.

### Row와 saveType

Canonical row type은 `N`, `I`, `U`, `D`, `O`입니다. XML의 생략된 `Row@type`과
JSON의 생략된 `_RowType_`은 `N`으로 decode합니다. Original row가 없으면 XML은
`OrgRow`를, JSON은 인접한 `O` row를 생략합니다. JSON/SSV의 `O` record는 바로
앞 materialized row의 `orgRow`가 됩니다.

`saveType`은 root와 Dataset에 선택적으로 존재합니다. Dataset 값이 root보다
우선합니다.

| 값 | 정책 |
|---:|---|
| `0` | root 정책 상속 |
| `1` | all |
| `2` | normal |
| `3` | updated |
| `4` | deleted |
| `5` | changed |

둘 다 없거나 0이면 encoder는 canonical row와 row type을 모두 보존합니다.

### Scalar

Type 이름은 대소문자를 구분하지 않습니다.

- STRING, CHAR
- SHORT, USHORT, INT, UINT, LONG, ULONG
- FLOAT, DOUBLE
- DECIMAL, BIGDECIMAL
- BOOLEAN
- DATE, TIME, DATETIME
- BLOB, FILE
- NULL

PlatformBinary는 wire 내부 tag인 UNDEFINED, DATASET, INVALID도 표현합니다.
BLOB과 FILE lexical 값은 canonical base64입니다.


Machine-readable 정의:

- [`canonical.schema.json`](canonical.schema.json)
- [`vector.schema.json`](vector.schema.json)
- [`request.schema.json`](request.schema.json)
- [`response.schema.json`](response.schema.json)
- [`capabilities.schema.json`](capabilities.schema.json)

## Schema와 wire 검증

`protocol/*.schema.json`은 모든 profile이 공유하는 JSON request/response,
vector manifest, canonical value 구조를 검증합니다. `input.data`와
`output.data` 안의 base64 wire bytes는 JSON Schema에서 의도적으로 opaque하게
취급합니다. 실제 profile grammar는 실행 가능한 vector로 검증합니다.

- XML: declaration, namespace, element/attribute, entity, row 구조
- JSON: Dataset field, type, property 순서, duplicate key
- SSV: stream header, RS/US, header, row, state marker, terminal null record
- Binary: signature, version, block framing, scalar tag, row, saved value,
  count, length
- PlatformZlib: 자동 decode와 profile별 encode/roundtrip 조합

Entity 처리는 profile별로 다릅니다.

- XML은 XML character/entity reference를 decode합니다. 필요한 line feed는
  encode 시 `&#10;`으로 표현합니다.
- JSON은 JSON string escaping만 적용합니다. `&amp;`, `&#10;` 같은 text는
  XML entity 처리를 하지 않고 literal로 유지합니다.
- SSV는 XML entity 처리를 하지 않으며 separator와 state-marker가 framing을
  결정합니다.

Strict mode는 필수 container grammar를 강제합니다. Compatibility mode
(`strict:false`)는 문서화된 empty-container 생략도 허용합니다. Encoder는
compatibility omission form을 사용합니다.

## 보안과 자원 제한

Adapter는 DTD와 external entity를 resolve하면 안 됩니다. Duplicate JSON key와
XML attribute를 거부하고, 제한 없는 메모리를 할당하기 전에 다음 제한을
적용해야 합니다.

- raw/decompressed payload bytes
- nesting depth
- Dataset 수
- row 수
- column 수
- scalar bytes
- BLOB bytes

비정상 XML, JSON, SSV, binary, 잘못된 base64는 process crash가 아니라 protocol
error로 반환해야 합니다.
