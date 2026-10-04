# Adapter 연동 가이드

어댑터(Adapter)는 테스트 대상 라이브러리(또는 드라이버)와 Conformance Runner 간의 통신을 중계하는 프로세스입니다.

어댑터는 대상 언어(JavaScript/TypeScript, Java, Go, Rust, Python 등)나 내부 데이터 구조(DTO)에 구애받지 않으며, [프로토콜 명세](../protocol/README.md)에 정의된 **표준 JSON 요청/응답(Envelope)**과 **Canonical 데이터 모델**만을 교환합니다.

---

## 통신 방식 (Transport)

어댑터는 환경에 따라 **stdio(JSON Lines 서브프로세스)** 또는 **HTTP 서버** 중 하나로 구현할 수 있습니다.

### 1. stdio 모드 (권장, CI/CD 최적화)

러너가 어댑터 프로세스를 서브프로세스로 직접 실행하고 표준 입출력(stdio)으로 통신합니다. 포트 충돌이 없고 프로세스 수명 주기가 자동으로 관리되므로 CI 및 로컬 테스트에 권장됩니다.

- **동작 규칙**:
  1. **초기화**: 러너가 실행 직후 첫 줄로 `{"operation":"capabilities"}`를 전달하면, 어댑터는 자신의 기능 명세(Capabilities) JSON을 stdout에 한 줄로 응답해야 합니다.
  2. **요청/응답**: 이후 러너가 전달하는 각 테스트 요청(JSONL 1줄)에 대해 정확히 1줄의 JSON 응답을 stdout으로 출력합니다.
  3. **로그 분리**: 디버깅 로그나 오류 메시지는 반드시 **stderr**로만 출력해야 합니다. (stdout에 로그가 섞이면 JSON 파싱 실패로 처리됩니다.)
  4. **종료**: stdin에 EOF가 수신되면 정상 종료합니다.

### 2. HTTP 모드

어댑터를 독립된 HTTP 서버로 구동하고 러너가 REST API 엔드포인트로 요청을 보냅니다. `curl`을 통한 수동 디버깅이나 원격 환경 테스트에 유용합니다.

- 기본 엔드포인트: `http://127.0.0.1:8787` (러너의 `-url` 옵션으로 변경 가능)
- 요청/응답 형식: `Content-Type: application/json`

---

## 필수 구현 연산

어댑터는 다음 4가지 핵심 연산을 처리해야 합니다:

| 연산 | HTTP 엔드포인트 | 설명 |
|---|---|---|
| `capabilities` | `GET /capabilities` | 지원하는 프로필, 연산, 옵션 및 제약 사항 목록 반환 |
| `decode` | `POST /decode` | Wire bytes(base64)를 파싱하여 Canonical JSON 데이터로 변환 |
| `encode` | `POST /encode` | Canonical JSON 데이터를 대상 프로필의 Wire bytes(base64)로 직렬화 |
| `roundtrip` | `POST /roundtrip` | Wire bytes를 decode 후 다시 encode하여 Canonical 값과 직렬화된 바이트를 모두 반환 |

*실제로 구현체에서 지원하는 프로필과 연산만 `capabilities` 응답에 포함해야 하며, 지원하지 않는 프로필/연산 요청을 받으면 적절한 에러(`unsupported-profile`, `unsupported-operation`)를 반환해야 합니다.*

---

## 요청 및 응답 예시

### 디코딩 요청 (`decode`)

```json
{
  "case": "xml.dataset.basic",
  "operation": "decode",
  "profile": "nexacro-xml-4000",
  "input": {
    "encoding": "base64",
    "data": "PD94bWwgdmVyc2lvbj0iMS4wIiBlbmNvZGluZz0idXRmLTgiPz4..."
  },
  "options": {
    "strict": true
  }
}
```

### 성공 응답

```json
{
  "ok": true,
  "value": {
    "parameters": [],
    "datasets": [
      {
        "id": "ds_sample",
        "columns": [
          { "id": "code", "type": "STRING", "index": 0 }
        ],
        "constColumns": [],
        "rows": [
          {
            "type": "N",
            "orgRow": null,
            "values": {
              "code": { "state": "value", "lexical": "A100" }
            }
          }
        ]
      }
    ]
  }
}
```

*`encode` 또는 `roundtrip` 연산의 경우, 위 응답의 `output` 필드에 인코딩된 Wire bytes(`{"encoding":"base64","data":"..."}`)가 포함됩니다.*

### 실패 응답

파싱 오류, 잘못된 형식, 제약 초과 등의 오류 발생 시 프로세스를 비정상 종료(crash)시키지 않고 정해진 에러 형식으로 응답해야 합니다.

```json
{
  "ok": false,
  "error": {
    "class": "malformed-input",
    "path": "Root.Dataset.Rows.Row.Col",
    "message": "Invalid XML closing tag"
  }
}
```

주요 에러 클래스(`class`):
- `invalid-request`: 필수 필드 누락 등 요청 자체의 문제
- `unsupported-operation` / `unsupported-profile`: 미지원 연산 또는 프로필
- `malformed-input`: 문법 오류, 잘못된 wire 형식
- `invalid-value`: 타입 불일치, 유효하지 않은 값
- `limit-exceeded`: 페이로드 크기나 row/col 수 등 리소스 한도 초과

---

## 어댑터 실행 및 검증 예시

### 1. stdio 모드로 검증 실행

```sh
# 자체 어댑터 (예: Node.js)를 서브프로세스로 실행하여 검증
go run ./cmd/xapi-runner -command 'node ./my-adapter.js stdio'

# 또는 npx로 실행
npx @clickin/xapi-conformance -command 'node ./my-adapter.js stdio'
```

### 2. HTTP 모드로 검증 실행

```sh
# 1) 어댑터 서버 구동
node ./my-adapter.js --port 8787

# 2) 러너 실행
npx @clickin/xapi-conformance -url http://127.0.0.1:8787
```

### 3. 필터링 및 옵션 활용

특정 프로필이나 연산만 집중적으로 디버깅할 때:

```sh
# nexacro-xml-4000 프로필의 decode 연산만 검증
npx @clickin/xapi-conformance \
  -command 'node ./my-adapter.js stdio' \
  -profile nexacro-xml-4000 \
  -operation decode \
  -filter "basic"
```

### 4. Go 레퍼런스 어댑터 실행

저장소에 내장된 Go 레퍼런스 어댑터 동작 확인:

```sh
# stdio 모드
go run ./cmd/xapi-runner -command 'go run ./cmd/xapi-reference stdio'

# HTTP 모드
go run ./cmd/xapi-reference
go run ./cmd/xapi-runner -url http://127.0.0.1:8787
```

---

## 구현 시 주의사항

1. **Canonical 표현 유지**: 숫자, 날짜/시간 값 등을 호스트 언어의 원시 타입으로 캐스팅하여 정밀도를 잃지 마십시오. 어댑터는 wire의 텍스트 표현(`lexical`)을 그대로 유지해야 합니다.
2. **stdout 오염 방지**: stdio 모드 사용 시 라이브러리 내부의 `console.log`, `System.out.println` 등의 로그 출력이 stdout으로 유입되지 않도록 로깅 설정을 stderr로 격리하십시오.
3. **타임아웃 준수**: 각 요청은 러너의 타임아웃(기본 10초) 내에 응답해야 합니다.
4. **상세 프로토콜 명세**: Canonical 데이터 구조 및 각 프로필별 상세 명세는 [프로토콜 명세 (`protocol/README.md`)](../protocol/README.md)를 참고하세요.
