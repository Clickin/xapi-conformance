# XPLATFORM / Nexacro Dataset Conformance Protocol

Protocol version: `1.0`

본 프로토콜은 특정 프로그래밍 언어나 라이브러리 구현체에 종속되지 않는 **Dataset Wire 프로토콜 검증 규격**입니다.

러너와 어댑터는 JSON Envelope 규격을 통해 통신하며, 테스트 대상 Wire 데이터(XML, JSON, SSV, 바이너리)는 base64로 인코딩되어 교환됩니다. 파싱된 데이터는 호스트 언어의 원시 타입으로 변환되지 않고 **표준 Canonical JSON 모델**로 표현되어 의미적 동등성을 검증합니다.

---

## 전송 계층 및 통신 인터페이스

어댑터는 환경에 따라 다음 두 가지 인터페이스 중 하나를 구현합니다:

1. **stdio (JSON Lines 서브프로세스, 권장)**:
   - 러너가 어댑터를 서브프로세스로 실행하고 표준 입출력으로 통신합니다.
   - 요청마다 `stdin`으로 한 줄의 JSON 객체를 전달하고, 어댑터는 `stdout`으로 정확히 한 줄의 JSON 응답을 반환합니다.
   - 모든 디버깅 및 진단 로그는 반드시 `stderr`로만 출력해야 합니다.
2. **HTTP (REST API)**:
   - 모든 요청 및 응답의 `Content-Type`은 `application/json`입니다.
   - 기본 포트는 `http://127.0.0.1:8787`입니다.

---

## 엔드포인트 및 연산

어댑터는 다음 4가지 핵심 연산을 처리합니다:

| 연산 | HTTP 엔드포인트 | 설명 |
|---|---|---|
| `capabilities` | `GET /capabilities` | 어댑터가 지원하는 프로필, 연산, 옵션 및 제약 목록 조회 |
| `decode` | `POST /decode` | 입력 Wire bytes를 Canonical JSON 데이터 모델로 파싱 |
| `encode` | `POST /encode` | Canonical JSON 데이터를 대상 프로필의 Wire bytes로 직렬화 |
| `roundtrip` | `POST /roundtrip` | Wire bytes를 decode 후 다시 encode하여 Canonical 값과 출력 bytes 모두 반환 |

---

## 요청 (Request)

```json
{
  "case": "xml.dataset.basic",
  "operation": "decode",
  "profile": "nexacro-xml-4000",
  "input": {
    "encoding": "base64",
    "data": "..."
  },
  "options": {
    "strict": true
  }
}
```

### 필드 명세

- `case`: 테스트 벡터 식별자 (문자열)
- `operation`: 수행할 연산 (`decode`, `encode`, `roundtrip` 중 하나)
- `profile`: 대상 프로필 명칭 (예: `nexacro-xml-4000`)
- `input`: 입력 Wire 데이터 (`decode`, `roundtrip` 시 필수, `{"encoding":"base64","data":"..."}`)
- `value`: 입력 Canonical 값 (`encode` 시 필수)
- `options`: 검증 및 동작 옵션 객체 (선택)

### 옵션 (`options`)

| 옵션 | 타입 | 기본값 | 설명 |
|---|---|---|---|
| `strict` | boolean | `false` | 엄격한 문법 검증 적용 (비표준 컨테이너 생략 불허 등) |
| `base64Whitespace` | boolean | `false` | base64 문자열 내 공백 문자 허용 여부 |
| `zlib` | boolean | `false` | `encode`/`roundtrip` 출력에 zlib 압축 전송 계층 적용 |
| `ssvUnitSeparator` | string | (기본값) | SSV Unit Separator (Unicode scalar 1자) |
| `ssvRecordSeparator` | string | (기본값) | SSV Record Separator (Unicode scalar 1자) |
| `limits` | object | `{}` | 리소스 제한 (`payloadBytes`, `datasets`, `rows`, `columns`, `scalarBytes`, `blobBytes`) |

---

## 성공 응답 (Success Response)

```json
{
  "ok": true,
  "value": {
    "parameters": [],
    "datasets": []
  },
  "output": {
    "encoding": "base64",
    "data": "..."
  }
}
```

- `ok`: 성공 여부 (`true`)
- `value`: 디코딩된 **Canonical 데이터 모델**입니다.
- `output`: `encode` 및 `roundtrip` 연산 성공 시 필수적으로 포함되는 결과 Wire bytes(base64)입니다. 줄바꿈 없는 순수 base64 문자열이어야 합니다.

---

## 실패 응답 (Error Response)

형식 오류, 제약 초과, 미지원 연산 등의 예외 상황에서는 프로세스를 비정상 종료(crash)하지 않고 오류 객체를 반환합니다:

```json
{
  "ok": false,
  "error": {
    "class": "malformed-input",
    "path": "Root.Dataset.Rows.Row.Col",
    "message": "진단 메시지"
  }
}
```

### 표준 에러 클래스 (`class`)

- `invalid-request`: 요청 스키마 위반, 필수 필드 누락, 지원하지 않는 옵션 지정
- `unsupported-operation`: 어댑터가 지원하지 않는 연산 요청
- `unsupported-profile`: 어댑터가 지원하지 않는 프로필 요청
- `malformed-input`: 문법 오류, 잘못된 base64, 깨진 wire 데이터
- `invalid-value`: 스키마/타입 정의에 어긋나는 유효하지 않은 값
- `limit-exceeded`: 지정된 페이로드 크기나 row/column 수 한도 초과
- `timeout`: 타임아웃 초과
- `internal`: 어댑터 내부 처리 오류

*러너는 `error.class`와 `error.path`를 기준으로 검증하며, 사람이 읽는 `message`는 진단용이므로 구현체 간 동일성을 요구하지 않습니다.*

---

## 기능 조회 (`capabilities`)

러너는 테스트 시작 전 `GET /capabilities` (또는 stdio의 `{"operation":"capabilities"}`)를 호출하여 어댑터가 지원하는 기능을 확인합니다:

```json
{
  "protocolVersion": "1.0",
  "implementation": "my-adapter",
  "profiles": [
    {
      "name": "nexacro-json-1.0",
      "operations": ["decode", "encode", "roundtrip"],
      "options": ["strict", "base64Whitespace", "limits", "zlib"],
      "limits": {
        "payloadBytes": 10485760
      }
    }
  ]
}
```

러너는 어댑터의 capabilities에 선언되지 않은 선택적 프로필/연산 벡터는 자동으로 건너뜁니다. 단, 필수(required) 테스트 벡터의 지원이 누락된 경우 실패로 보고합니다.

---

## 지원 프로필

| 프로필 | 프로토콜 명칭 | 기본 지원 연산 |
|---|---|---|
| `nexacro-json-1.0` | Nexacro Dataset JSON 1.0 | decode, encode, roundtrip |
| `nexacro-xml-4000` | Nexacro Platform XML 4000 | decode, encode, roundtrip |
| `xplatform-xml-4000` | XPLATFORM XML 4000 | decode, encode, roundtrip |
| `nexacro-ssv` | Nexacro SSV | decode, encode, roundtrip |
| `xplatform-ssv` | XPLATFORM SSV | decode, encode, roundtrip |
| `nexacro-binary-5000` | Nexacro PlatformBinary 5000 | decode, encode, roundtrip |
| `xplatform-binary-5000` | XPLATFORM PlatformBinary 5000 | decode, encode, roundtrip |

*모든 프로필은 `options.zlib`을 통해 zlib 압축 전송 계층과 조합하여 검증할 수 있습니다.*

---

## Canonical 데이터 모델

Canonical 모델은 프로그래밍 언어의 원시 타입으로 변환하는 과정에서 발생할 수 있는 정밀도 손실이나 왜곡 없이, Wire 상의 원래 의미와 텍스트 표현(`lexical`)을 보존합니다.

```json
{
  "parameters": [
    {
      "id": "ErrorCode",
      "type": "INT",
      "lexical": "0",
      "state": "value"
    }
  ],
  "datasets": [
    {
      "id": "output",
      "columns": [
        { "id": "stockCode", "type": "STRING", "index": 0 }
      ],
      "constColumns": [],
      "rows": [
        {
          "type": "N",
          "orgRow": null,
          "values": {
            "stockCode": {
              "state": "value",
              "lexical": "10001"
            }
          }
        }
      ]
    }
  ]
}
```

- **순서 보존**: Parameter와 Column의 wire 순서는 `index` 필드로 보존됩니다.
- **키 순서 무관**: JSON object key의 순서는 비교 대상이 아닙니다. Dataset Column은 `id`를 기준으로 조회되므로 순서가 달라도 의미가 같으면 동일하게 판정합니다.

### 셀 상태 (`state`)

- `value`: 정상 값이 존재하며 `lexical` 문자열이 필수입니다.
- `null`: 명시적인 null 상태입니다.
- `empty`: 빈 값이며 `lexical`은 `""`입니다.
- `missing`: wire 상에 해당 필드 또는 셀이 생략된 상태입니다.

### Row 타입 및 `saveType`

- **Row 타입**: `N` (Normal), `I` (Inserted), `U` (Updated), `D` (Deleted), `O` (Original).
  - XML의 생략된 `Row@type`과 JSON의 생략된 `_RowType_`은 `N`으로 디코딩됩니다.
  - JSON/SSV의 `O` 레코드는 직전 인접 행의 `orgRow`로 매핑됩니다.
- **`saveType`**: 루트 또는 Dataset 단위의 저장 정책입니다 (Dataset 값이 우선):
  - `0`: 루트 정책 상속 또는 기본 보존
  - `1`: all (전체 행)
  - `2`: normal (일반 행)
  - `3`: updated (수정된 행)
  - `4`: deleted (삭제된 행)
  - `5`: changed (수정/삭제/추가된 행)

### 지원 스칼라 타입

타입 명칭은 대소문자를 구분하지 않습니다:

- 문자열: `STRING`, `CHAR`
- 정수: `SHORT`, `USHORT`, `INT`, `UINT`, `LONG`, `ULONG`
- 부동소수점/실수: `FLOAT`, `DOUBLE`, `DECIMAL`, `BIGDECIMAL`
- 논리: `BOOLEAN`
- 날짜/시간: `DATE`, `TIME`, `DATETIME`
- 바이너리/파일: `BLOB`, `FILE` (lexical 값은 canonical base64 문자열)
- 기타: `NULL`, 바이너리 내부 태그(`UNDEFINED`, `DATASET`, `INVALID`)

---

## 포맷별 Wire 처리 규칙

- **XML**: XML 선언, 네임스페이스, 엘리먼트/속성 구조, 문자 참조(`&#10;` 줄바꿈 등), CDATA 구문을 검증합니다.
- **JSON**: Dataset 필드 구조, 속성 순서, 중복 키 거부, JSON 문자열 이스케이프를 검증합니다. (XML 엔티티는 변환하지 않고 리터럴로 유지)
- **SSV**: 스트림 헤더, RS(0x1E)/US(0x1F) 구분자, 헤더 및 데이터 레코드, 상태 마커를 검증합니다.
- **바이너리**: 매직 시그니처, 5000 버전 헤더, 블록 프레이밍, 스칼라 태그, 길이 및 레코드 개수를 검증합니다.
- **zlib 압축**: `FF AD` 시그니처로 시작하는 페이로드를 자동 감지하여 디코딩하며, `options.zlib` 활성화 시 인코딩 결과에 압축을 적용합니다.

---

## 스키마 파일 목록

JSON Envelope 및 데이터 구조에 대한 Machine-readable 명세:

- [`canonical.schema.json`](canonical.schema.json): Canonical 데이터 모델 스키마
- [`vector.schema.json`](vector.schema.json): 테스트 벡터 매니페스트 스키마
- [`request.schema.json`](request.schema.json): 요청 Envelope 스키마
- [`response.schema.json`](response.schema.json): 응답 Envelope 스키마
- [`capabilities.schema.json`](capabilities.schema.json): 기능 선언 스키마

---

## 보안 및 리소스 제약

어댑터는 악의적인 페이로드나 비정상 입력에 대해 다음과 같은 보호 조치를 적용해야 합니다:

- **외부 엔티티 참조 방지**: XML DTD 및 외부 엔티티(XXE) 참조를 비활성화해야 합니다.
- **중복 키/속성 거부**: 중복된 JSON 키 또는 XML 속성을 허용하지 않아야 합니다.
- **리소스 한도 검사**: 과도한 메모리 할당을 방지하기 위해 페이로드 크기, 중첩 깊이, Dataset/Row/Column 수, 스칼라 및 BLOB 크기 제한을 적용해야 합니다.
- **프로토콜 에러 반환**: 비정상적인 입력이나 유효하지 않은 base64 데이터에 대해 프로세스 크래시 없이 표준 에러 응답(`malformed-input` 등)을 반환해야 합니다.
