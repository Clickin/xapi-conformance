# 원본 driver 출력 vector (`vectors/driver`)

`vectors/driver/**`의 기대값은 사람이 쓴 것이 아니라 원본 XPLATFORM/Nexacro
Java driver가 실제로 쓰고 읽은 결과입니다. 문서와 driver 출력이 다르면 driver
출력을 기준으로 삼습니다.

## 구성

| 경로 | 내용 |
|---|---|
| `capture/mkcases.py` | case 파일 생성기. `python3 capture/mkcases.py capture/cases` |
| `capture/cases/*.json` | case 목록. `id`, `profile`, `operation`, `value` 또는 `text`/`base64` |
| `cmd/xapi-capture` | `capture`: case를 driver adapter에 보내 vector 작성. `diff`: 무작위 값으로 driver와 다른 adapter 비교 |
| `vectors/driver/<group>/` | 생성된 vector (escape, types, empty, savetype, binary, scalar, const) |

Driver adapter(`meta/xapi-original-adapter`)는 라이선스가 있는 driver jar를
쓰므로 저장소에 넣지 않습니다(`meta/`는 gitignore). Driver는 `TZ=UTC`로
실행합니다. PlatformBinary는 날짜를 epoch millisecond로 쓰므로 vector와 구현의
날짜 변환은 모두 UTC 기준입니다.

```sh
go run ./cmd/xapi-capture capture -cases capture/cases -out vectors/driver
go run ./cmd/xapi-capture diff -n 500 -seed 11 -other http://127.0.0.1:8791
```

## Case 옵션

- `alsoDecode`: encode 결과를 같은 driver로 다시 decode한 결과를
  `<id>.decode` vector로 함께 씁니다. driver가 자기 출력을 거부하면 error
  vector가 됩니다.
- `faithful`: driver가 자기 출력을 다른 값으로 읽으면 case를 건너뜁니다.
  driver가 정보를 잃는 출력은 구현에 강제하지 않기 위해서입니다. 건너뛴
  case는 capture 실행 시 `skip` 줄로 나옵니다 (현재 27건: Nexacro XML의 CR/CRLF와
  제어 문자, SSV 구분자 문자, `\x03`, Nexacro constant의 raw tab/LF/CR 등).

## 재현하지 않는 driver 동작

- 시간만 있는 lexical을 DATE/DATETIME으로 읽거나 PlatformBinary에 TIME
  constant를 쓰면 driver는 현재 날짜를 씁니다. 재현할 수 없으므로 vector에서
  제외하고, reference는 1970-01-01을 씁니다.
- PlatformBinary stream이 빈 문자열/BLOB 값으로 끝나면 driver decode가
  `Range [-1, -1 + 1) out of bounds`로 실패합니다. Stream 끝 처리 버그로 보고
  재현하지 않습니다.
- Dataset/column id는 escape 없이 쓰여 XML이 깨지므로 id escape case는
  두지 않습니다.

## 구현에 요구하는 driver 동작

일반적이지 않지만 vector로 고정한 동작입니다.

- encode는 `saveType`을 적용합니다 (`protocol/README.md`의 Row 타입 및
  `saveType`). 기본값은 NORMAL(2)이고 `D` 행은 Dataset 끝에 씁니다.
- 텍스트 형식의 BLOB parameter는 wire 텍스트 bytes의 base64를 lexical로
  씁니다.
- JSON column `size`는 문자열이어야 합니다. 숫자 `size`는 decode error입니다.
- JSON constant 값은 decode 시 변환하지 않고 문자열 그대로 둡니다. XML/SSV
  constant는 column 타입으로 변환합니다.
- 텍스트 형식 encode에서 constant 값은 BOOLEAN만 `1`/`0`으로 바꾸고(빈 값과
  null은 `0`), 나머지는 주어진 그대로 씁니다. 빈 constant는 `value=""`로
  쓰고, null constant는 값 없이 씁니다 (SSV는 `=` 뒤를 비움).
- PlatformBinary constant는 column 타입으로 변환해 씁니다. 빈 값은 숫자 타입에서
  0, BIGDECIMAL/날짜 타입에서 null이 되고, primitive 타입의 null은 0입니다.
- 값이 없는 BLOB `ConstColumn`은 driver가 직접 쓰지만 XML decode에서는
  거부합니다.
- 원본 행이 있는 `U` 행에서 생략된 셀은 원본 값을 씁니다.
- 숫자/날짜 lexical은 driver의 `StringTypeConverter` 규칙으로 변환합니다
  (`internal/codec/javascalar.go`). Java `Double.toString`/`BigDecimal.toString`
  표기, 날짜 길이별 layout, 자릿수 넘침의 lenient 정규화를 따릅니다.
- PlatformBinary NaN은 `0x7ff8000000000000`으로 씁니다.

## XML encoder

Reference XML encoder는 줄 단위로 직접 bytes를 씁니다. `xmltext` Encoder로
바꾸는 방안은 driver의 attribute escape(XPLATFORM의 `&#N;`, `&#32;`)와
self-closing 형식이 섞인 출력(BLOB constant의 `></ConstColumn>`)을 재현할 수
없어 채택하지 않았습니다. XML decode는 `xmltext`를 씁니다.

## Decode `wire.root`

Driver API는 decode한 문서의 root namespace를 알려주지 않습니다. Decode 결과의
`wire.root`는 입력에서 관찰한 값이 아니라 profile별로 정해진 값입니다
(XPLATFORM `http://www.tobesoft.com/platform/Dataset`, `ver` 5000). Encode는
driver와 같이 `http://www.tobesoft.com/platform/dataset`를 씁니다.
