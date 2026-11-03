package httpplan

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
