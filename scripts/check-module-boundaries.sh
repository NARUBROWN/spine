#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# 각 공개 계약 패키지는 표준 라이브러리만 사용하며 서로와 커널 구현에도 의존하지 않습니다.
for contract in adapter/http adapter/event adapter; do
  contract_path="github.com/NARUBROWN/spine/$contract"
  external=$(go list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' "./$contract")
  unexpected=$(printf '%s\n' "$external" | awk -v contract_path="$contract_path" -v legacy="$contract" '
    legacy == "adapter" && $0 == "github.com/NARUBROWN/spine/adapter/http" { next }
    NF && $0 != contract_path && $0 != contract_path ".test" &&
      $0 != contract_path " [" contract_path ".test]" &&
      $0 != contract_path "_test [" contract_path ".test]"
  ')
  if [ -n "$unexpected" ]; then
    printf '%s contract has external dependencies:\n%s\n' "$contract" "$unexpected" >&2
    exit 1
  fi
done

# 커널은 구체 HTTP 구현을 참조하지 않습니다. 브로커 구현은 계약 연결 후 분리합니다.
kernel=$(go list -deps -test ./...)
if printf '%s\n' "$kernel" | grep -E '^(github.com/labstack/|github.com/NARUBROWN/spine-echo-platform)' ; then
  printf 'Kernel depends on an HTTP implementation.\n' >&2
  exit 1
fi
