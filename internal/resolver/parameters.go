package resolver

import (
	"reflect"

	"github.com/NARUBROWN/spine/pkg/path"
)

// Parameters는 실행과 명세가 같은 순서로 경로 키를 배정하도록 합니다.
func Parameters(method reflect.Method, pathKeys []string) []ParameterMeta {
	result := make([]ParameterMeta, 0, method.Type.NumIn()-1)
	pathIndex := 0
	for i := 1; i < method.Type.NumIn(); i++ {
		t := method.Type.In(i)
		parameter := ParameterMeta{Index: i - 1, Type: t}
		if t.PkgPath() == reflect.TypeFor[path.Int]().PkgPath() {
			if pathIndex < len(pathKeys) {
				parameter.PathKey = pathKeys[pathIndex]
			}
			pathIndex++
		}
		result = append(result, parameter)
	}
	return result
}
