package boot

import "github.com/NARUBROWN/spine/internal/configuration"

// ConfigIssue는 app.Validate와 Prepare가 보고하는 설정 문제입니다.
type ConfigIssue = configuration.ConfigIssue

// ConfigError는 설정 문제의 목록입니다. errors.As로 조회할 수 있습니다.
type ConfigError = configuration.ConfigError
