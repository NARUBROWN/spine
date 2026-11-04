package endpoint

import (
	"fmt"
	"strings"
)

func Path(prefix, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("route path cannot be empty")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path, nil
}

// PathKeys는 라우터와 같은 구간 규칙으로 이름을 읽고 중복되거나 빈 이름을 거부합니다.
func PathKeys(route string) ([]string, error) {
	var keys []string
	seen := map[string]bool{}
	for _, segment := range strings.Split(route, "/") {
		if !strings.HasPrefix(segment, ":") {
			continue
		}
		name := segment[1:]
		if name == "" || strings.ContainsAny(name, "{}:*?#\\") || strings.ContainsAny(name, " \t\r\n") {
			return nil, fmt.Errorf("invalid path parameter name %q", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("route repeats path parameter %q", name)
		}
		seen[name] = true
		keys = append(keys, name)
	}
	return keys, nil
}
