#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# 공개 계약 패키지는 표준 라이브러리만 사용하고 커널은 구체 HTTP 구현을 참조하지 않습니다.
external=$(go list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./adapter)
unexpected=$(printf '%s\n' "$external" | awk 'NF && index($0, "github.com/NARUBROWN/spine/adapter") != 1')
if [ -n "$unexpected" ]; then
  printf 'Contract has external dependencies:\n%s\n' "$unexpected" >&2
  exit 1
fi

kernel=$(go list -deps -test ./...)
if printf '%s\n' "$kernel" | grep -E '^(github.com/labstack/|github.com/NARUBROWN/spine-echo-platform)' ; then
  printf 'Kernel depends on an HTTP implementation.\n' >&2
  exit 1
fi
