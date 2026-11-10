package endpoint

import (
	"fmt"

	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// CallableIssues는 새 콜백의 입력과 반환을 리스너 시작 전에 검사합니다.
func CallableIssues(meta execution.HandlerMeta, prefix string) []boot.ConfigIssue {
	var issues []boot.ConfigIssue
	for _, parameter := range resolver.HandlerParameters(meta, nil) {
		supported := false
		for _, candidate := range Resolvers(nil) {
			if candidate.Supports(parameter) {
				supported = true
				break
			}
		}
		if !supported {
			issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Parameters[%d]", prefix, parameter.Index), Code: "HTTP_PARAMETER_UNSUPPORTED", Message: fmt.Sprintf("No resolver supports %s.", parameter.Type), Hint: "Use input.Body[T], input.Query[T], input.Header[T], input.Path[T] or a supported context."})
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
			issues = append(issues, boot.ConfigIssue{Path: fmt.Sprintf("%s.Returns[%d]", prefix, index), Code: "HTTP_RETURN_UNSUPPORTED", Message: fmt.Sprintf("No return handler supports %s.", t.Out(index)), Hint: "Use httpx.Response[T] or another supported return type."})
		}
	}
	return issues
}
