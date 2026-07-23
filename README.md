# xapi-conformance

XPLATFORM/Nexacro Dataset의 XML, JSON, SSV 및 바이너리 wire 형식을 검증하는
언어 중립 conformance 벡터와 HTTP 계약입니다.

구현체는 작은 HTTP 서비스를 노출합니다. Runner는 실제 wire payload를 전송하고
결과를 언어 중립 canonical 표현과 비교합니다. 의미 비교 대상인 XML/JSON은
바이트 단위로 비교하지 않고 decode 및 normalize한 뒤 비교합니다. 정확한 출력이
계약인 벡터는 wire bytes를 그대로 비교합니다.

## 계약

필수 endpoint:

- `GET /capabilities`
- `POST /decode`
- `POST /encode`
- `POST /roundtrip`

전체 wire 계약은 [`protocol/README.md`](protocol/README.md)에 정의되어 있습니다.

## npm 또는 pnpm으로 실행

npm 패키지에는 플랫폼별 runner 실행 파일이 포함되어 있습니다. 설치 과정에서
코드를 내려받거나 컴파일하지 않으며 `install`/`postinstall` script도 사용하지
않습니다.

지원 대상:

- macOS arm64, x64
- Linux arm64, x64
- Windows arm64, x64

Node.js wrapper가 `process.platform`과 `process.arch`에 맞는 실행 파일을 선택합니다.

```sh
pnpm dlx @clickin/xapi-conformance -url http://127.0.0.1:8787
npx @clickin/xapi-conformance -url http://127.0.0.1:8787
```

Wrapper는 runner의 모든 flag를 그대로 전달합니다. 기본적으로 패키지에 포함된
벡터를 사용하며 `-vectors`로 다른 벡터 디렉터리를 지정할 수 있습니다.

## 저장소에서 검증 실행

CI와 동일한 검증:

```sh
go test ./...
go run ./cmd/xapi-validate -vectors vectors
go run ./cmd/xapi-runner -command 'go run ./cmd/xapi-reference stdio' \
  -json results.json -junit results.xml
```

HTTP adapter를 직접 실행하는 방법:

```sh
go run ./cmd/xapi-reference
go run ./cmd/xapi-runner -url http://127.0.0.1:8787
```

Runner는 `-profile`, `-operation`, `-parallel`, `-timeout`, `-filter`를
지원합니다. 벡터의 expectation 종류에 따라 canonical JSON 또는 정확한 wire
출력을 비교합니다.

## 지원 범위

다음 wire profile을 검증합니다.

- Nexacro Dataset JSON 1.0
- Nexacro/XPLATFORM XML 4000
- Nexacro/XPLATFORM SSV
- Nexacro/XPLATFORM PlatformBinary 5000
- 모든 profile과 조합 가능한 PlatformZlib transport

Scalar 및 내부 wire type, lexical 변환, null/missing/empty, parameter,
Dataset, constant, 모든 row state, original/removed row, `saveType`, 순서 변경,
optional-field powerset, BLOB/FILE, 비정상 입력, 자원 제한, strict/compatibility
정책을 포함합니다.

대형 종합 fixture는 회귀 검증용으로만 유지합니다. 조합 완전성은 독립적인
per-mask 벡터로 검증합니다. 세부 범위는 [`docs/matrix.md`](docs/matrix.md),
실행 결과는 [`docs/audit-findings.md`](docs/audit-findings.md)를 참고하십시오.

## Fixture 출처

일부 fixture와 assertion은 Clickin의 `xapi-js`, `xplatform-xml`, `xapi`
저장소에서 가져옵니다. 가져온 항목은 원본 저장소, commit, path, license 정보를
유지합니다. 자세한 내용은 [`sources/README.md`](sources/README.md)와
[`sources/ATTRIBUTIONS.md`](sources/ATTRIBUTIONS.md)에 있습니다.
