package jsondecode

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"unicode"
)

type jsonField struct {
	name   string
	typeOf reflect.Type
	index  []int
	tagged bool
	quoted bool
}

type jsonFields struct {
	exact   map[string]jsonField
	ordered []jsonField
}

func (f jsonFields) find(name string) (jsonField, bool) {
	if field, ok := f.exact[name]; ok {
		return field, true
	}
	for _, field := range f.ordered {
		if strings.EqualFold(field.name, name) {
			return field, true
		}
	}
	return jsonField{}, false
}

var fieldCache sync.Map

// fieldsFor는 JSON 태그, 익명 필드 우선순위와 대소문자 일치 규칙을 보존합니다.
// 알려지지 않은 키나 충돌로 숨겨진 필드는 encoding/json과 같이 무시합니다.
func fieldsFor(root reflect.Type) jsonFields {
	if cached, ok := fieldCache.Load(root); ok {
		return cached.(jsonFields)
	}
	type entry struct {
		t       reflect.Type
		index   []int
		visited map[reflect.Type]bool
	}
	queue := []entry{{root, nil, map[reflect.Type]bool{root: true}}}
	byName := map[string][]jsonField{}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		for i := 0; i < e.t.NumField(); i++ {
			field := e.t.Field(i)
			base := field.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if !field.IsExported() && (!field.Anonymous || base.Kind() != reflect.Struct) {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, options, _ := strings.Cut(tag, ",")
			if !validName(name) {
				name = ""
			}
			index := append(append([]int(nil), e.index...), i)
			if field.Anonymous && base.Kind() == reflect.Struct && name == "" {
				if e.visited[base] {
					continue
				}
				visited := make(map[reflect.Type]bool, len(e.visited)+1)
				for typ := range e.visited {
					visited[typ] = true
				}
				visited[base] = true
				queue = append(queue, entry{base, index, visited})
				continue
			}
			candidate := jsonField{name: name, typeOf: field.Type, index: index, tagged: name != ""}
			if name == "" {
				candidate.name = field.Name
			}
			for _, option := range strings.Split(options, ",") {
				if option == "string" {
					kind := base.Kind()
					candidate.quoted = kind == reflect.String || kind == reflect.Bool || kind >= reflect.Int && kind <= reflect.Float64
				}
			}
			byName[candidate.name] = append(byName[candidate.name], candidate)
		}
	}
	result := jsonFields{exact: map[string]jsonField{}}
	for _, candidates := range byName {
		slices.SortStableFunc(candidates, func(a, b jsonField) int {
			if depth := len(a.index) - len(b.index); depth != 0 {
				return depth
			}
			if a.tagged == b.tagged {
				return 0
			}
			if a.tagged {
				return -1
			}
			return 1
		})
		winner := candidates[0]
		if len(candidates) > 1 && len(winner.index) == len(candidates[1].index) && winner.tagged == candidates[1].tagged {
			continue
		}
		result.exact[winner.name] = winner
		result.ordered = append(result.ordered, winner)
	}
	slices.SortFunc(result.ordered, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	stored, _ := fieldCache.LoadOrStore(root, result)
	return stored.(jsonFields)
}

func validName(name string) bool {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", r) {
			return false
		}
	}
	return true
}
