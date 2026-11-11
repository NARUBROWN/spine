package endpoint

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/NARUBROWN/spine/internal/configuration"
	"github.com/NARUBROWN/spine/internal/execution"
	"github.com/NARUBROWN/spine/internal/input"
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
	if len(issues) == 0 {
		return ResponseTypeIssues(meta, prefix)
	}
	return issues
}

// ResponseTypeIssues는 정적으로 판별 가능한 HTTP 응답 본문 타입 오류를 사전에 검사합니다.
func ResponseTypeIssues(meta execution.HandlerMeta, prefix string) []configuration.ConfigIssue {
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
		if !exists {
			continue
		}
		if body.Type.Kind() == reflect.String && body.Type != reflect.TypeFor[string]() {
			issues = append(issues, configuration.ConfigIssue{
				Path: fmt.Sprintf("%s.Returns[%d]", prefix, index), Code: "HTTP_RETURN_UNSUPPORTED",
				Message: fmt.Sprintf("httpx.Response does not support defined string body %s.", body.Type),
				Hint:    "Return httpx.Response[string] and convert Body with string(value); struct DTO responses and string aliases remain supported.",
			})
			continue
		}
		if unsupported := unsupportedJSONType(body.Type, false, make(map[jsonTypeUse]bool)); unsupported != nil {
			issues = append(issues, configuration.ConfigIssue{
				Path: fmt.Sprintf("%s.Returns[%d]", prefix, index), Code: "HTTP_RETURN_UNSUPPORTED",
				Message: fmt.Sprintf("httpx.Response body %s contains unsupported JSON type %s.", body.Type, unsupported),
				Hint:    "Use a JSON-compatible type or implement JSON serialization.",
			})
		}
	}
	return issues
}

type jsonTypeUse struct {
	typeOf      reflect.Type
	addressable bool
}

func unsupportedJSONType(t reflect.Type, addressable bool, seen map[jsonTypeUse]bool) reflect.Type {
	use := jsonTypeUse{typeOf: t, addressable: addressable}
	if seen[use] {
		return nil
	}
	seen[use] = true
	for _, marshaler := range []reflect.Type{reflect.TypeFor[json.Marshaler](), reflect.TypeFor[encoding.TextMarshaler]()} {
		if t.Implements(marshaler) || addressable && reflect.PointerTo(t).Implements(marshaler) {
			return nil
		}
	}
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Complex64, reflect.Complex128, reflect.UnsafePointer:
		return t
	case reflect.Map:
		key := t.Key()
		if key.Kind() != reflect.String && !integerKind(key.Kind()) && !key.Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
			return t
		}
		return unsupportedJSONType(t.Elem(), false, seen)
	case reflect.Pointer, reflect.Slice:
		return unsupportedJSONType(t.Elem(), true, seen)
	case reflect.Array:
		return unsupportedJSONType(t.Elem(), addressable, seen)
	case reflect.Struct:
		fields, _ := input.BodyFields(t, false)
		for _, field := range fields {
			if unsupported := unsupportedJSONType(field.Type, addressable || field.Indirect, seen); unsupported != nil {
				return unsupported
			}
		}
	}
	return nil
}
