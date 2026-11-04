package endpoint

import (
	"reflect"
	"strconv"
	"strings"
)

// typeIdentity는 익명 구조체 안에서도 동명 패키지의 서로 다른 타입을 구별합니다.
func typeIdentity(t reflect.Type) string {
	if t.Name() != "" {
		return strconv.Quote(t.PkgPath()) + ":" + strconv.Quote(t.Name())
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		return t.Kind().String() + "(" + typeIdentity(t.Elem()) + ")"
	case reflect.Array:
		return "array[" + strconv.Itoa(t.Len()) + "]" + typeIdentity(t.Elem())
	case reflect.Map:
		return "map[" + typeIdentity(t.Key()) + "]" + typeIdentity(t.Elem())
	case reflect.Struct:
		var result strings.Builder
		result.WriteString("struct{")
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			result.WriteString(strconv.Quote(f.Name))
			result.WriteString(strconv.Quote(f.PkgPath))
			result.WriteString(strconv.Quote(string(f.Tag)))
			result.WriteString(strconv.FormatBool(f.Anonymous))
			result.WriteString(typeIdentity(f.Type))
			result.WriteByte(';')
		}
		result.WriteByte('}')
		return result.String()
	}
	return t.String()
}
