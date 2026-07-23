# Adapter 계약 가이드

Adapter는 [`protocol/README.md`](../protocol/README.md)의 HTTP 계약을 구현하는
process입니다. 내부 DTO는 자유롭게 선택할 수 있지만 구현체 사이에서 공유하는
형식은 protocol envelope와 canonical JSON뿐입니다.

## 필수 동작

Adapter는 다음 endpoint를 구현합니다.

- `GET /capabilities`
- `POST /decode`
- `POST /encode`
- `POST /roundtrip`

실제로 지원하는 profile, operation, option만 capability에 노출해야 합니다.
Decode 전에 payload limit을 적용하고 오류는 `ok:false`와 안정적인 error
`class`/`path`로 반환합니다.

## Go reference adapter

```sh
go run ./cmd/xapi-reference
go run ./cmd/xapi-runner -url http://127.0.0.1:8787
```

## 다른 언어 구현

Java, JavaScript, Rust 등으로 구현한 adapter도 같은 네 endpoint와 canonical
model을 사용합니다. Runner는 vector의 expectation에 따라 canonical 의미 비교
또는 exact-wire 비교를 수행합니다. 구현체 CI에서는 이 저장소의 commit이나 tag를
고정해 사용하십시오. 특정 언어의 protocol DTO를 복사해 공통 계약으로 사용하면
안 됩니다.

Adapter transport는 HTTP 또는 JSONL subprocess 중 하나를 선택할 수 있습니다.
JSONL mode 규칙:

1. 첫 요청 `{"operation":"capabilities"}`에 HTTP capability와 같은 객체로 응답합니다.
2. 이후 stdin에서 UTF-8 JSON envelope 한 줄을 읽습니다.
3. stdout에 JSON 응답을 정확히 한 줄 씁니다.
4. log는 stderr에만 씁니다.
5. EOF 뒤 종료합니다.

HTTP 요청 예시:

```sh
curl -s http://127.0.0.1:8787/decode \
  -H 'content-type: application/json' \
  -d '{"operation":"decode","profile":"nexacro-json-1.0","input":{"encoding":"base64","data":"eyJ2ZXJzaW9uIjoiMS4wIiwiUGFyYW1ldGVycyI6W10sIkRhdGFzZXRzIjpbXX0="}}'
```

고정된 CI 실행에는 JSONL subprocess mode가 빠르고 안정적입니다. Runner는
process timeout, 조기 종료, 요청/응답 개수 불일치를 실패로 처리합니다.

```sh
go run ./cmd/xapi-runner \
  -command 'go run ./cmd/xapi-reference stdio' \
  -timeout 10s
```

`-profile`, `-operation`으로 범위를 고정할 수 있습니다.

```sh
-profile nexacro-xml-4000 -operation decode
```
