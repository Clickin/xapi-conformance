# Fixture 출처 inventory

세 디렉터리는 fixture 분석과 provenance 확인에 사용하는 shallow clone입니다.
Adapter runtime dependency가 아니며 해당 저장소의 구현 type이 HTTP protocol
경계를 통과하지 않습니다.

| 저장소 | 고정 commit | 관련 test 영역 |
|---|---|---|
| [Clickin/xapi-js](https://github.com/Clickin/xapi-js) | `244339098838d098d7087588d039a24d26448b5e` | `packages/core/test/{codec,handler,schema,xapi-data}.test.ts` |
| [Clickin/xplatform-xml](https://github.com/Clickin/xplatform-xml) | `cda8b6b31f64511ff9d22f64539c4b78e862455d` | `xplatform-xml-micronaut/src/test/.../XplatformXmlSerdeTest.java`, processor test |
| [Clickin/xapi](https://github.com/Clickin/xapi) | `e0d54759162bbb06aef8a1bc4c557f9fe7336991` | `xapi-json/src/test`, `xapi-xml/src/test/resources/xapi` |

Inventory 기준일은 2026-07-21입니다. 가져온 vector의 `source` object는 정확한
commit과 path를 가리켜야 합니다. Machine-readable count와 변환 상태는
[`inventory.json`](inventory.json)에 있습니다.

`cmd/xapi-import`는 고정된 XML fixture와 test 안의 XML literal을 vector로
변환하면서 commit/path/license provenance를 보존합니다. Malformed fixture는
`vectors/invalid/imported` 아래 negative vector가 됩니다.

Type-level schema test, framework adapter mock, annotation processor diagnostic처럼
wire를 통과하지 않는 assertion은 Dataset wire vector로 가장하지 않습니다.
독립적으로 분리할 수 있는 wire literal만 deterministic vector로 가져옵니다.
