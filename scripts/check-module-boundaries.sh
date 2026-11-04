#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# 각 공개 계약 패키지는 표준 라이브러리만 사용하며 서로와 커널 구현에도 의존하지 않습니다.
for contract in contract/description contract/http contract/event contract/validator contract/websocket adapter/http adapter/event adapter/websocket adapter; do
  contract_path="github.com/NARUBROWN/spine/$contract"
  external=$(go run ./scripts/test list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' "./$contract")
  unexpected=$(printf '%s\n' "$external" | awk -v contract_path="$contract_path" -v legacy="$contract" '
    (legacy == "adapter" || legacy == "adapter/http") && $0 == "github.com/NARUBROWN/spine/contract/http" { next }
    legacy == "adapter" && $0 == "github.com/NARUBROWN/spine/adapter/http" { next }
    legacy == "adapter/event" && $0 == "github.com/NARUBROWN/spine/contract/event" { next }
    legacy == "adapter/websocket" && $0 == "github.com/NARUBROWN/spine/contract/websocket" { next }
    NF && $0 != contract_path && $0 != contract_path ".test" &&
      $0 != contract_path " [" contract_path ".test]" &&
      $0 != contract_path "_test [" contract_path ".test]"
  ')
  if [ -n "$unexpected" ]; then
    printf '%s contract has external dependencies:\n%s\n' "$contract" "$unexpected" >&2
    exit 1
  fi
done

# 커널과 계약은 외부 모듈 없이 표준 라이브러리만 사용합니다.
kernel=$(go run ./scripts/test list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./...)
unexpected=$(printf '%s\n' "$kernel" | awk 'NF && $0 !~ /^github.com\/NARUBROWN\/spine([\/._ ]|$)/')
if [ -n "$unexpected" ]; then
  printf '%s\n' "$unexpected" >&2
  printf 'Kernel depends on an external module.\n' >&2
  exit 1
fi
