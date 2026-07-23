# Conformance vector

각 vector는 `protocol/vector.schema.json`으로 정의한 JSON manifest입니다.
Schema는 manifest와 base64 transport envelope를 검증하며, 내부 XML/JSON/SSV
wire 문서 자체를 검증하지 않습니다. 큰 payload는 인접한 `.xml`, `.json`,
`.ssv` 파일에 저장하고 manifest에서 상대 경로로 참조할 수 있습니다.

`expect.kind`:

- `canonical`: decode한 canonical 의미 비교
- `wire`: base64 output envelope 정확 비교
- `error`: error class와 선택적인 path 비교

모든 vector는 protocol 필수 여부 또는 구현체 확장 여부를 명시해야 합니다.
또한 `source.repository`, `source.commit`, `source.path`, `source.license`를
포함해야 하며 가져온 fixture는 원본 attribution도 유지합니다.

Runner 실행 전에 validator로 schema, manifest metadata, base64를 검사하십시오.

```sh
go run ./cmd/xapi-validate -vectors vectors
```

JSON Schema는 embedded wire grammar를 검사하지 않습니다. 각 profile의 parser와
encoder 계약은 runner가 vector를 실제로 실행하여 검증합니다.
