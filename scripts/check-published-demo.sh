#!/usr/bin/env bash
set -euo pipefail

# 실제 demo를 저장소 밖에서 실행해 로컬 소스가 공개 모듈 검증에 섞이지 않게 합니다.
repo_root=$(cd "$(dirname "$0")/.." && pwd)
workspace=$(mktemp -d "${TMPDIR:-/tmp}/spine-published-demo.XXXXXX")
trap 'rm -rf "$workspace"' EXIT
cp -R "$repo_root/cmd/demo" "$workspace/demo"
mkdir -p "$workspace/demo/test/unit"
cp -R "$repo_root/test/unit/cmd/demo/." "$workspace/demo/test/unit/"
cp "$repo_root/test/unit/go.mod" "$workspace/demo/test/unit/go.mod"
cd "$workspace/demo"

export GOWORK=off
export GOMODCACHE="$workspace/modules"
export GOFLAGS=-modcacherw

spine_version=$(awk '$1 == "github.com/NARUBROWN/spine" { print $2; exit }' go.mod)
gorilla_version=$(awk '$1 == "github.com/NARUBROWN/spine-gorilla-platform" { print $2; exit }' go.mod)
platform_version=$(awk '$1 == "github.com/NARUBROWN/spine-echo-platform" { print $2; exit }' go.mod)
go mod edit -dropreplace=github.com/NARUBROWN/spine
go mod edit -dropreplace=github.com/NARUBROWN/spine-echo-platform
# 플랫폼의 전이 의존성만으로 필요한 Spine 버전도 선택되는지 확인합니다.
go mod edit -droprequire=github.com/NARUBROWN/spine
go mod tidy
go mod verify

replacements=$(go list -m -f '{{if .Replace}}{{.Path}}{{end}}' all)
if [ -n "$replacements" ]; then
  printf 'Public module verification cannot use replacements:\n%s\n' "$replacements" >&2
  exit 1
fi

for module in github.com/NARUBROWN/spine github.com/NARUBROWN/spine-echo-platform github.com/NARUBROWN/spine-gorilla-platform; do
  case "$module" in
    github.com/NARUBROWN/spine) expected="$spine_version" ;;
    github.com/NARUBROWN/spine-echo-platform) expected="$platform_version" ;;
    github.com/NARUBROWN/spine-gorilla-platform) expected="$gorilla_version" ;;
  esac
  actual=$(go list -m -f '{{.Version}}' "$module")
  if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
    printf 'Unexpected module version for %s: expected=%s actual=%s\n' "$module" "$expected" "$actual" >&2
    exit 1
  fi
  printf '%s %s\n' "$module" "$actual"
done

go run "$repo_root/scripts/test/main.go" -root "$workspace/demo" vet ./...
# 같은 BuildApplication과 기존 HTTP/WS, SIGINT 및 Prepare E2E를 사용합니다.
go run "$repo_root/scripts/test/main.go" -root "$workspace/demo" test -race ./... -count=1 -timeout=3m
