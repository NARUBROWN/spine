package resolver

import (
	"github.com/NARUBROWN/spine/core"
	"reflect"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/path"
	"github.com/NARUBROWN/spine/pkg/query"
)

// Parameters는 실행과 명세가 같은 순서로 경로 키를 배정하도록 합니다.
func Parameters(method reflect.Method, pathKeys []string) []ParameterMeta {
	return callableParameters(method.Type, 1, pathKeys)
}

func HandlerParameters(meta core.HandlerMeta, pathKeys []string) []ParameterMeta {
	return callableParameters(meta.FunctionType(), meta.ArgumentOffset(), pathKeys)
}

func callableParameters(signature reflect.Type, offset int, pathKeys []string) []ParameterMeta {
	result := make([]ParameterMeta, 0, signature.NumIn()-offset)
	pathIndex := 0
	for i := offset; i < signature.NumIn(); i++ {
		t := signature.In(i)
		parameter := ParameterMeta{Index: i - offset, Type: t}
		if IsLegacyPath(t) {
			if pathIndex < len(pathKeys) {
				parameter.PathKey = pathKeys[pathIndex]
			}
			pathIndex++
		}
		result = append(result, parameter)
	}
	return result
}

// IsLegacyPath는 전환 기간에만 사용하는 기존 경로 래퍼를 식별합니다.
func IsLegacyPath(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == reflect.TypeFor[path.Int]() || t == reflect.TypeFor[path.String]() || t == reflect.TypeFor[path.Boolean]()
}

// IsLegacyValues는 Deprecated 쿼리와 헤더 조회 타입을 식별합니다.
func IsLegacyValues(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == reflect.TypeFor[query.Values]() || t == reflect.TypeFor[header.Values]()
}

// IsLegacyPagination은 Deprecated 페이지네이션 입력을 식별합니다.
func IsLegacyPagination(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t == reflect.TypeFor[query.Pagination]()
}
