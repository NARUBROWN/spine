package configuration

import (
	"fmt"
	"strings"
)

// ConfigIssue는 설정값의 비밀 정보를 노출하지 않으면서 사용자가 조치할 수 있는
// 하나의 설정 문제를 설명합니다.
type ConfigIssue struct {
	Path    string
	Code    string
	Message string
	Hint    string
}

// ConfigError는 연결을 하나씩 시도하다 실패하는 대신 호출자가 한 번의 사전 검사로
// 모든 문제를 보고할 수 있도록 설정 문제를 묶습니다.
type ConfigError struct {
	Issues []ConfigIssue
}

func (e *ConfigError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "configuration validation failed"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "configuration validation failed with %d issue(s):", len(e.Issues))
	for _, issue := range e.Issues {
		fmt.Fprintf(&b, "\n- %s [%s]: %s", issue.Path, issue.Code, issue.Message)
		if issue.Hint != "" {
			fmt.Fprintf(&b, "\n  Hint: %s", issue.Hint)
		}
	}
	return b.String()
}

// ValidationError는 문제 목록이 비어 있으면 nil을, 그렇지 않으면 ConfigError를 반환합니다.
// 독립적인 사전 검사 검증기를 조합할 때 유용합니다.
func ValidationError(issues ...ConfigIssue) error {
	if len(issues) == 0 {
		return nil
	}
	return &ConfigError{Issues: append([]ConfigIssue(nil), issues...)}
}

func configPath(prefix, field string) string {
	if prefix == "" {
		return field
	}
	if field == "" {
		return prefix
	}
	return prefix + "." + field
}
