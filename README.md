# xapi-conformance

XPLATFORM 및 Nexacro Dataset wire 프로토콜(XML, JSON, SSV, 바이너리) 및 압축 전송(zlib 등)의 호환성을 검증하는 **독립적인 Conformance Test Suite & Runner**입니다.

특정 프로그래밍 언어나 특정 라이브러리 구현체에 종속되지 않으며, 구현체가 제공하는 간단한 어댑터(HTTP 또는 stdio)를 통해 wire 데이터의 인코딩·디코딩 정합성 및 표준 동작을 객관적으로 검증합니다.

---

## 주요 특징

- **표준 Wire 프로토콜 검증**: Nexacro XML 4000, XPLATFORM XML 4000, JSON 1.0, SSV, PlatformBinary 5000 및 zlib 압축 전송 지원
- **언어 중립적 검증 아키텍처**: HTTP REST 엔드포인트 또는 stdio(JSONL 서브프로세스)를 통한 표준 어댑터 계약
- **체계적인 검증 수준**:
  - `canonical`: 파싱 결과의 의미적 구조(Dataset, Column, Row, Type, State 등) 일치 검증
  - `wire`: 엄격한 wire-bytes 수준의 일치 검증
  - `error`: 비정상 입력, 형식 오류, 제약 초과에 대한 적절한 에러 반환 검증
- **테스트 러너 내장**: 필터링, 동시성 병렬 실행, JUnit XML / JSON 리포트 출력 지원 (CI 연동 용이)

---

## 지원 프로필

| 프로필 | 프로토콜 명칭 | 지원 연산 |
|---|---|---|
| `nexacro-json-1.0` | Nexacro Dataset JSON 1.0 | decode, encode, roundtrip |
| `nexacro-xml-4000` | Nexacro Platform XML 4000 | decode, encode, roundtrip |
| `xplatform-xml-4000` | XPLATFORM XML 4000 | decode, encode, roundtrip |
| `nexacro-ssv` | Nexacro SSV | decode, encode, roundtrip |
| `xplatform-ssv` | XPLATFORM SSV | decode, encode, roundtrip |
| `nexacro-binary-5000` | Nexacro PlatformBinary 5000 | decode, encode, roundtrip |
| `xplatform-binary-5000` | XPLATFORM PlatformBinary 5000 | decode, encode, roundtrip |

*모든 프로필은 zlib 압축 옵션(`options.zlib`)과 조합하여 검증할 수 있습니다.*

---

## 빠른 시작

npm 패키지(`@clickin/xapi-conformance`)에는 주요 OS 및 아키텍처(macOS, Linux, Windows - arm64/x64)용 사전 빌드 바이너리가 번들되어 있어 별도의 빌드 단계 없이 즉시 실행할 수 있습니다.

```sh
# HTTP 어댑터 대상 실행
npx @clickin/xapi-conformance -url http://127.0.0.1:8787
# 또는
pnpm dlx @clickin/xapi-conformance -url http://127.0.0.1:8787

# stdio 서브프로세스 어댑터 대상 실행 (CI 환경 권장)
npx @clickin/xapi-conformance -command 'node ./my-adapter.js stdio'
```

---

## CLI 옵션

`xapi-conformance` (또는 Go 러너 `cmd/xapi-runner`)에서 지원하는 주요 옵션입니다:

| 옵션 | 기본값 | 설명 |
|---|---|---|
| `-url` | `http://127.0.0.1:8787` | 대상 HTTP 어댑터 URL |
| `-command` | `""` | 실행할 서브프로세스 어댑터 명령 (stdio JSONL 모드) |
| `-vectors` | `vectors` | 테스트 벡터 디렉터리 경로 |
| `-profile` | `""` | 특정 프로필만 필터링 (예: `nexacro-xml-4000`) |
| `-operation` | `""` | 특정 연산만 필터링 (`decode`, `encode`, `roundtrip`) |
| `-suite` | `all` | 테스트 스위트 필터 (`all`, `vendor`, `security`, `limitation`) |
| `-filter` | `""` | 벡터 ID 부분 문자열 필터 |
| `-parallel` | `1` | 동시 요청 수 |
| `-timeout` | `10s` | 요청별 제한 시간 |
| `-json` | `""` | 실행 결과를 저장할 JSON 파일 경로 |
| `-junit` | `""` | CI 연동용 JUnit XML 리포트 파일 경로 |

---

## 어댑터 연동 규격

자체 구현체를 검증하려면 러너와 통신할 수 있는 어댑터를 구현해야 합니다. 어댑터는 독립적인 **HTTP 서버** 또는 **stdio JSONL 서브프로세스** 형태로 동작할 수 있습니다.

### 필수 연산

1. **`capabilities`** (`GET /capabilities`): 지원하는 프로필, 연산, 옵션 목록 조회
2. **`decode`** (`POST /decode`): Wire bytes(base64)를 표준 canonical JSON으로 파싱
3. **`encode`** (`POST /encode`): 표준 canonical JSON을 Wire bytes(base64)로 직렬화
4. **`roundtrip`** (`POST /roundtrip`): 디코딩 후 재인코딩하여 canonical 값과 wire bytes를 함께 검증

자세한 메시지 스키마 및 통신 규약은 다음 문서를 참고하세요:

- [프로토콜 명세 (`protocol/README.md`)](protocol/README.md)
- [어댑터 작성 가이드 (`adapters/README.md`)](adapters/README.md)

---

## 저장소 개발 및 검증

저장소 자체의 테스트 및 레퍼런스 어댑터 실행:

```sh
# Go 단위 테스트 실행
go test ./...

# 테스트 벡터 유효성 검사 (JSON Schema 및 base64 무결성)
go run ./cmd/xapi-validate -vectors vectors

# Go 레퍼런스 어댑터에 대해 전체 검증 실행
go run ./cmd/xapi-runner -command 'go run ./cmd/xapi-reference stdio'
```

---

## 라이선스

[MIT License](LICENSE)
