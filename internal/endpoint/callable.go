package endpoint

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
)

// CallableIssues는 새 콜백의 입력과 반환을 리스너 시작 전에 검사합니다.
func CallableIssues(meta execution.HandlerMeta, prefix string) []configuration.ConfigIssue {
	var issues []configuration.ConfigIssue
	for _, parameter := range resolver.HandlerParameters(meta, nil) {
		supported := false
		for _, candidate := range Resolvers(nil) {
			if candidate.Supports(parameter) {
				supported = true
				break
			}
		}
		if !supported {
			issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_PARAMETER_UNSUPPORTED", Message: fmt.Sprintf("No resolver supports %s.", parameter.Type), Hint: "Use input.Body[T], input.Query[T], input.Header[T], input.Path[T] or a supported context."})
		}
	}
	t := meta.FunctionType()
	for index := 0; index < t.NumOut(); index++ {
		supported := false
		for _, candidate := range Returns() {
			if candidate.Supports(t.Out(index)) {
				supported = true
				break
			}
		}
		if !supported {
			issues = append(issues, configuration.ConfigIssue{Path: fmt.Sprintf("%s.Returns[%d]", prefix, index), Code: "HTTP_RETURN_UNSUPPORTED", Message: fmt.Sprintf("No return handler supports %s.", t.Out(index)), Hint: "Use httpx.Response[T] or another supported return type."})
		}
	}
	return issues
}

// StringResponseIssues는 실행과 명세가 지원하지 않는 정의 문자열 응답을 사전에 거부합니다.
// struct DTO와 기본 문자열의 별칭은 기존 반환 계약을 유지합니다.
func StringResponseIssues(meta execution.HandlerMeta, prefix string) []configuration.ConfigIssue {
	var issues []configuration.ConfigIssue
	signature := meta.FunctionType()
	for index := 0; index < signature.NumOut(); index++ {
		result := signature.Out(index)
		if result.Kind() == reflect.Pointer {
			result = result.Elem()
		}
		if result.Kind() != reflect.Struct || result.PkgPath() != "github.com/NARUBROWN/spine/pkg/httpx" || !strings.HasPrefix(result.Name(), "Response[") {
			continue
		}
		body, exists := result.FieldByName("Body")
		if exists && body.Type.Kind() == reflect.String && body.Type != reflect.TypeFor[string]() {
			issues = append(issues, configuration.ConfigIssue{
				Path: fmt.Sprintf("%s.Returns[%d]", prefix, index), Code: "HTTP_RETURN_UNSUPPORTED",
				Message: fmt.Sprintf("httpx.Response does not support defined string body %s.", body.Type),
				Hint:    "Return httpx.Response[string] and convert Body with string(value); struct DTO responses and string aliases remain supported.",
			})
		}
	}
	return issues
}
