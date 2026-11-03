// Package apidesc는 등록된 타입을 부작용 없이 HTTP 설명으로 변환합니다.
package apidesc

import (
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
)

type schemaKey struct {
	typeOf reflect.Type
	mode   string
}

type catalog struct {
	schemas     map[string]any
	ids         map[schemaKey]string
	validator   validation.Validator
	diagnostics []description.Diagnostic
	seen        map[string]bool
}

func newCatalog(validator validation.Validator) *catalog {
	return &catalog{schemas: map[string]any{}, ids: map[schemaKey]string{}, validator: validator, seen: map[string]bool{}}
}

func (c *catalog) diagnostic(code, target, message string) {
	key := code + "\x00" + target + "\x00" + message
	if !c.seen[key] {
		c.seen[key] = true
		c.diagnostics = append(c.diagnostics, description.Diagnostic{Code: code, Target: target, Message: message})
	}
}

func (c *catalog) schema(t reflect.Type, mode string) any {
	if t.Kind() == reflect.Pointer {
		seen := map[reflect.Type]bool{}
		for current := t; current.Kind() == reflect.Pointer; current = current.Elem() {
			if seen[current] {
				c.diagnostic("RECURSIVE_POINTER_UNRESOLVED", t.String(), "A pointer-only cycle has no finite object schema.")
				return map[string]any{}
			}
			seen[current] = true
		}
	}
	if t.Name() != "" && (t.Kind() == reflect.Slice || t.Kind() == reflect.Map || t.Kind() == reflect.Array) && t != reflect.TypeFor[json.RawMessage]() && !customCodec(t) {
		key := schemaKey{t, mode}
		if id, exists := c.ids[key]; exists {
			return reference(id)
		}
		hash := sha256.Sum256([]byte(t.PkgPath() + ":" + t.Name() + ":" + mode))
		id := fmt.Sprintf("Collection_%x", hash[:8])
		c.ids[key], c.schemas[id] = id, map[string]any{}
		c.schemas[id] = c.schemaValue(t, mode)
		return reference(id)
	}
	return c.schemaValue(t, mode)
}

func (c *catalog) schemaValue(t reflect.Type, mode string) any {
	if t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	if t == reflect.TypeFor[json.Number]() {
		return map[string]any{"type": "number"}
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		return nullable(c.schema(t.Elem(), mode))
	}
	if customCodec(t) {
		c.diagnostic("CUSTOM_SERIALIZATION_UNRESOLVED", t.String(), "Custom JSON or text serialization requires an explicit schema.")
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Int, reflect.Uint, reflect.Int8, reflect.Uint8, reflect.Int16, reflect.Uint16, reflect.Int32, reflect.Uint32, reflect.Int64, reflect.Uint64, reflect.Uintptr:
		bits := uint(t.Bits())
		maximum := new(big.Int).Lsh(big.NewInt(1), bits)
		minimum := big.NewInt(0)
		if t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64 {
			maximum.Rsh(maximum, 1)
			minimum.Neg(new(big.Int).Set(maximum))
		}
		maximum.Sub(maximum, big.NewInt(1))
		result := map[string]any{"type": "integer", "minimum": json.Number(minimum.String()), "maximum": json.Number(maximum.String())}
		if t.Kind() == reflect.Int64 {
			result["format"] = "int64"
		}
		return result
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		c.diagnostic("DYNAMIC_TYPE_UNRESOLVED", t.String(), "An interface does not describe its runtime JSON representation.")
		return map[string]any{}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nullable(map[string]any{"type": "string", "contentEncoding": "base64"})
		}
		return nullable(map[string]any{"type": "array", "items": c.schema(t.Elem(), mode)})
	case reflect.Array:
		return map[string]any{"type": "array", "items": c.schema(t.Elem(), mode), "minItems": t.Len(), "maxItems": t.Len()}
	case reflect.Map:
		key := t.Key()
		if key.Kind() != reflect.String && !integerKind(key.Kind()) && !key.Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
			c.diagnostic("MAP_KEY_UNRESOLVED", t.String(), "The map key has no supported JSON object representation.")
			return map[string]any{}
		}
		return nullable(map[string]any{"type": "object", "additionalProperties": c.schema(t.Elem(), mode)})
	case reflect.Struct:
		key := schemaKey{t, mode}
		if id, ok := c.ids[key]; ok {
			return reference(id)
		}
		base := t.Name()
		if base == "" {
			base = "Object"
		}
		base = strings.Map(func(r rune) rune {
			if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
				return r
			}
			return '_'
		}, base)
		hash := sha256.Sum256([]byte(t.PkgPath() + ":" + t.String() + ":" + mode))
		id := fmt.Sprintf("%s_%x", base, hash[:8])
		c.ids[key], c.schemas[id] = id, map[string]any{}
		c.schemas[id] = c.structure(t, mode)
		return reference(id)
	}
	c.diagnostic("TYPE_UNRESOLVED", t.String(), "This type has no supported JSON schema.")
	return map[string]any{}
}

func integerKind(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Uintptr }
func nullable(schema any) any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}
func reference(id string) any { return map[string]any{"$ref": "#/components/schemas/" + id} }

func customCodecType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() {
		return false
	}
	return customCodec(t)
}

func customCodec(t reflect.Type) bool {
	for _, codec := range []reflect.Type{reflect.TypeFor[json.Marshaler](), reflect.TypeFor[json.Unmarshaler](), reflect.TypeFor[encoding.TextMarshaler](), reflect.TypeFor[encoding.TextUnmarshaler]()} {
		if t.Implements(codec) || reflect.PointerTo(t).Implements(codec) {
			return true
		}
	}
	return false
}

func (c *catalog) structure(t reflect.Type, mode string) any {
	fields := wireFields(t, strings.HasSuffix(mode, "/form"))
	constraints := map[string]validation.FieldSchema{}
	if strings.HasPrefix(mode, "request/") && c.validator != nil {
		if describer, ok := c.validator.(validation.SchemaDescriber); ok {
			decl, err := describeValidation(describer, t)
			if err != nil {
				c.diagnostic("VALIDATION_DESCRIPTION_FAILED", t.String(), "The registered validator could not describe this type.")
			} else {
				for _, field := range decl.Fields {
					constraints[fmt.Sprint(field.Index)] = field
				}
				for _, rule := range decl.Unresolved {
					c.diagnostic("VALIDATION_RULE_UNRESOLVED", t.String(), rule)
				}
			}
		} else {
			c.diagnostic("VALIDATION_DESCRIPTION_UNAVAILABLE", t.String(), "The registered validator does not expose validation rules.")
		}
	}
	properties := map[string]any{}
	required := []string{}
	for _, field := range fields {
		constraint, exists := constraints[fmt.Sprint(field.index)]
		fieldMode := mode
		if constraint.SkipNested {
			fieldMode = "unvalidated/" + strings.TrimPrefix(mode, "request/")
		}
		var schema any = c.schema(field.typeOf, fieldMode)
		if field.quoted {
			schema = map[string]any{"type": "string"}
			if field.typeOf.Kind() == reflect.Pointer {
				schema = nullable(schema)
			}
		}
		if exists {
			var value map[string]any
			decoder := json.NewDecoder(strings.NewReader(string(constraint.Schema)))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil || !json.Valid(constraint.Schema) || value == nil || field.quoted || customCodecType(field.typeOf) {
				c.diagnostic("VALIDATION_SCHEMA_UNRESOLVED", t.String()+"."+field.name, "The field constraint has no supported wire schema.")
			} else {
				schema = map[string]any{"allOf": []any{schema, value}}
				if constraint.Required {
					required = append(required, field.name)
				}
			}
		}
		if strings.HasPrefix(mode, "response/") && !field.optional && !field.indirect {
			required = append(required, field.name)
		}
		properties[field.name] = schema
	}
	sort.Strings(required)
	result := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func describeValidation(describer validation.SchemaDescriber, t reflect.Type) (result validation.SchemaDescription, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("validation description panicked")
		}
	}()
	return describer.DescribeSchema(t)
}

type wireField struct {
	name                               string
	index                              []int
	typeOf                             reflect.Type
	tagged, optional, quoted, indirect bool
}

// wireFields는 익명 임베딩의 깊이, 태그 우선순위와 이름 충돌을 JSON과 같은 순서로 해석합니다.
// 폼에서는 form 태그가 있는 필드만 기술합니다.
func wireFields(root reflect.Type, form bool) []wireField {
	type entry struct {
		t        reflect.Type
		index    []int
		visited  map[reflect.Type]bool
		indirect bool
	}
	queue := []entry{{root, nil, map[reflect.Type]bool{root: true}, false}}
	byName := map[string][]wireField{}
	for len(queue) > 0 {
		e := queue[0]
		queue = queue[1:]
		for i := 0; i < e.t.NumField(); i++ {
			f := e.t.Field(i)
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
			name := parts[0]
			if tag == "-" {
				continue
			}
			if !validName(name) {
				name = ""
			}
			index := append(append([]int(nil), e.index...), i)
			if f.Anonymous && underlying.Kind() == reflect.Struct && name == "" {
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
			field := wireField{name: name, index: index, typeOf: f.Type, tagged: name != "", indirect: e.indirect}
			if field.name == "" {
				field.name = f.Name
			}
			for _, option := range parts[1:] {
				field.optional = field.optional || option == "omitempty" || option == "omitzero"
				field.quoted = field.quoted || option == "string"
			}
			kind := underlying.Kind()
			field.quoted = field.quoted && (kind == reflect.String || kind == reflect.Bool || integerKind(kind) || kind == reflect.Float32 || kind == reflect.Float64)
			byName[field.name] = append(byName[field.name], field)
		}
	}
	result := []wireField{}
	for _, candidates := range byName {
		sort.SliceStable(candidates, func(i, j int) bool {
			if len(candidates[i].index) != len(candidates[j].index) {
				return len(candidates[i].index) < len(candidates[j].index)
			}
			return candidates[i].tagged && !candidates[j].tagged
		})
		winner := candidates[0]
		if len(candidates) > 1 && len(winner.index) == len(candidates[1].index) && winner.tagged == candidates[1].tagged {
			continue
		}
		result = append(result, winner)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func validName(name string) bool {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", r) {
			return false
		}
	}
	return true
}

func raw(value any) json.RawMessage { contents, _ := json.Marshal(value); return contents }
