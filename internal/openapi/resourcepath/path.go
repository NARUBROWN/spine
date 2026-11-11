package resourcepath

import (
	"path"
	"strings"
	"unicode"
)

// Valid는 명세와 UI 리소스에 사용할 정적 경로를 검사합니다.
func Valid(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.ContainsAny(p, ":*?#%\\\r\n\t ") && path.Clean(p) == p && strings.IndexFunc(p, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
}
