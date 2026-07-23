# 출처와 license

Provenance가 없는 fixture는 corpus에 복사하지 않습니다. Checkout한 fixture
디렉터리는 원본 저장소 파일과 license notice를 유지합니다.

- `xapi-js`: MIT, copyright Robert Soriano (`xapi-js/LICENSE` 참고)
- `xapi`: Apache License 2.0 (`xapi/README.md`의 License 절 참고)
- `xplatform-xml`: 고정된 checkout에 `LICENSE`, `COPYING`, `NOTICE`가 없습니다.
  파생 wire vector는 정확한 commit/path와 `upstream-no-license-file` marker를
  유지하며 license를 임의로 추론하지 않습니다. 추가 fixture 재배포 정책은
  upstream 확인이 필요합니다.

이 저장소에서 새로 작성한 protocol과 runner code는 저장소 소유자가 별도로
license합니다. Provenance는 metadata이며 upstream DTO dependency가 아닙니다.
