package input

import (
	"reflect"
	"sort"
	"strings"
	"unicode"
)

type BodyField struct {
	Name                               string
	Index                              []int
	Type                               reflect.Type
	Tagged, Optional, Quoted, Indirect bool
}

// BodyFields는 JSON 이름과 폼의 펼친 필드를 명세와 사전 검사에 함께 제공합니다.
// 반환하는 충돌 이름은 입력 선언에서 거부하고, 응답에서는 Go JSON처럼 생략할 수 있습니다.
func BodyFields(root reflect.Type, form bool) ([]BodyField, []string) {
	type entry struct {
		t        reflect.Type
		index    []int
		visited  map[reflect.Type]bool
		indirect bool
	}
	queue := []entry{{root, nil, map[reflect.Type]bool{root: true}, false}}
	byName := map[string][]BodyField{}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		for i := 0; i < e.t.NumField(); i++ {
			f := e.t.Field(i)
			if form && !f.IsExported() {
				continue
			}
			underlying := f.Type
			if underlying.Kind() == reflect.Pointer {
				underlying = underlying.Elem()
			}
			if !f.IsExported() && (!f.Anonymous || underlying.Kind() != reflect.Struct) {
				continue
			}
			tag := f.Tag.Get("json")
			if form {
				tag = f.Tag.Get("form")
			}
			parts := strings.Split(tag, ",")
			if form {
				parts = []string{tag}
			}
			name := parts[0]
			if tag == "-" {
				continue
			}
			if !form && !validName(name) {
				name = ""
			}
			index := append(append([]int(nil), e.index...), i)
			if underlying.Kind() == reflect.Struct && name == "" && (!form && f.Anonymous || form && f.Type.Kind() == reflect.Struct) {
				if e.visited[underlying] {
					continue
				}
				seen := map[reflect.Type]bool{}
				for k, v := range e.visited {
					seen[k] = v
				}
				seen[underlying] = true
				queue = append(queue, entry{underlying, index, seen, e.indirect || f.Type.Kind() == reflect.Pointer})
				continue
			}
			if form && name == "" {
				continue
			}
			field := BodyField{Name: name, Index: index, Type: f.Type, Tagged: name != "", Indirect: e.indirect}
			if field.Name == "" {
				field.Name = f.Name
			}
			for _, option := range parts[1:] {
				field.Optional = field.Optional || option == "omitempty" || option == "omitzero"
				field.Quoted = field.Quoted || option == "string"
			}
			kind := underlying.Kind()
			field.Quoted = field.Quoted && (kind == reflect.String || kind == reflect.Bool || (kind >= reflect.Int && kind <= reflect.Uintptr) || kind == reflect.Float32 || kind == reflect.Float64)
			byName[field.Name] = append(byName[field.Name], field)
		}
	}
	result := []BodyField{}
	var conflicts []string
	for _, candidates := range byName {
		sort.SliceStable(candidates, func(i, j int) bool {
			if len(candidates[i].Index) != len(candidates[j].Index) {
				return len(candidates[i].Index) < len(candidates[j].Index)
			}
			return candidates[i].Tagged && !candidates[j].Tagged
		})
		winner := candidates[0]
		if len(candidates) > 1 && (form || len(winner.Index) == len(candidates[1].Index) && winner.Tagged == candidates[1].Tagged) {
			conflicts = append(conflicts, winner.Name)
			continue
		}
		result = append(result, winner)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	sort.Strings(conflicts)
	return result, conflicts
}

func validName(name string) bool {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", r) {
			return false
		}
	}
	return true
}
