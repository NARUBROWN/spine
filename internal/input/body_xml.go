package input

import (
	"encoding/xml"
	"reflect"
	"slices"
	"strings"
)

type xmlBodyField struct {
	index     []int
	path      []string
	namespace string
	mode      string
}

// checkXMLBody는 사용자 디코더를 실행하지 않고 XML 경로 충돌을 검사합니다.
// XML의 이름, 네임스페이스, 속성과 임베딩 우선순위는 JSON과 별도로 판정합니다.
func checkXMLBody(root reflect.Type) error {
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type) error
	walk = func(t reflect.Type) error {
		if seen[t] {
			return nil
		}
		seen[t] = true
		if t.Kind() != reflect.Pointer && bodyDecoder(reflect.PointerTo(t), "xml") {
			return nil
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice:
			return walk(t.Elem())
		case reflect.Struct:
			fields, err := xmlBodyFields(t, map[reflect.Type]bool{})
			if err != nil {
				return err
			}
			for _, field := range fields {
				if field.mode == "element" || field.mode == "element,any" {
					if err := walk(t.FieldByIndex(field.index).Type); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return walk(root)
}

func xmlBodyFields(t reflect.Type, visiting map[reflect.Type]bool) ([]xmlBodyField, error) {
	if visiting[t] || t == reflect.TypeFor[xml.Name]() {
		return nil, nil
	}
	visiting[t] = true
	defer delete(visiting, t)
	var fields []xmlBodyField
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() && !field.Anonymous || field.Tag.Get("xml") == "-" || field.Name == "XMLName" {
			continue
		}
		typ := field.Type
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		candidates := []xmlBodyField{xmlBodyFieldInfo(field)}
		if field.Anonymous && typ.Kind() == reflect.Struct {
			embedded, err := xmlBodyFields(typ, visiting)
			if err != nil {
				return nil, err
			}
			candidates = embedded
			for j := range candidates {
				candidates[j].index = append([]int{i}, candidates[j].index...)
			}
		}
		for _, candidate := range candidates {
			var conflicts []int
			for j, existing := range fields {
				if xmlBodyFieldsConflict(existing, candidate) {
					conflicts = append(conflicts, j)
				}
			}
			if slices.ContainsFunc(conflicts, func(j int) bool { return len(fields[j].index) < len(candidate.index) }) {
				continue // 더 얕은 필드가 임베딩한 필드보다 우선합니다.
			}
			for _, j := range conflicts {
				if len(fields[j].index) == len(candidate.index) {
					first, second := t.FieldByIndex(fields[j].index), t.FieldByIndex(candidate.index)
					return nil, &xml.TagPathError{Struct: t, Field1: first.Name, Tag1: first.Tag.Get("xml"), Field2: second.Name, Tag2: second.Tag.Get("xml")}
				}
			}
			for _, j := range slices.Backward(conflicts) {
				fields = slices.Delete(fields, j, j+1)
			}
			fields = append(fields, candidate)
		}
	}
	return fields, nil
}

func xmlBodyFieldInfo(field reflect.StructField) xmlBodyField {
	tag := field.Tag.Get("xml")
	var namespace string
	if ns, rest, ok := strings.Cut(tag, " "); ok {
		namespace, tag = ns, rest
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	var modes []string
	for _, mode := range []string{"attr", "cdata", "chardata", "innerxml", "comment", "any"} {
		if slices.Contains(parts[1:], mode) {
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 || len(modes) == 1 && modes[0] == "any" {
		modes = append([]string{"element"}, modes...)
	}
	if name == "" {
		name = field.Name
		typ := field.Type
		seen := map[reflect.Type]bool{}
		for typ.Kind() == reflect.Pointer && !seen[typ] {
			seen[typ] = true
			typ = typ.Elem()
		}
		if typ.Kind() == reflect.Struct {
			// encoding/xml는 필드 이름을 생략하면 직접 선언한 XMLName을 사용합니다.
			for i := 0; i < typ.NumField(); i++ {
				if inner := typ.Field(i); inner.Name == "XMLName" {
					xmlName, _, _ := strings.Cut(inner.Tag.Get("xml"), ",")
					ns, local, namespaced := strings.Cut(xmlName, " ")
					if local != "" && namespaced {
						namespace, name = ns, local
					} else if xmlName != "" && xmlName != "-" && !namespaced {
						name = xmlName
					}
					break
				}
			}
		}
		return xmlBodyField{index: field.Index, path: []string{name}, namespace: namespace, mode: strings.Join(modes, ",")}
	}
	path := strings.Split(name, ">")
	if path[0] == "" {
		path[0] = field.Name
	}
	return xmlBodyField{index: field.Index, path: path, namespace: namespace, mode: strings.Join(modes, ",")}
}

func xmlBodyFieldsConflict(a, b xmlBodyField) bool {
	if a.mode != b.mode || a.namespace != "" && b.namespace != "" && a.namespace != b.namespace {
		return false
	}
	length := min(len(a.path), len(b.path))
	if !slices.Equal(a.path[:length], b.path[:length]) {
		return false
	}
	return len(a.path) != len(b.path) || a.namespace == b.namespace
}
