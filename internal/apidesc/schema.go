// Package apidesc는 등록된 타입을 부작용 없이 HTTP 설명으로 변환합니다.
package apidesc

import (
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"math/big"
	"mime/multipart"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/input"
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
		hash := sha256.Sum256([]byte(typeIdentity(t) + ":" + mode))
		id := fmt.Sprintf("Collection_%x", hash[:8])
		c.ids[key], c.schemas[id] = id, map[string]any{}
		c.schemas[id] = c.schemaValue(t, mode)
		return reference(id)
	}
	return c.schemaValue(t, mode)
}

func (c *catalog) schemaValue(t reflect.Type, mode string) any {
	if strings.HasSuffix(mode, "/form") {
		if t == reflect.TypeFor[*multipart.FileHeader]() {
			return map[string]any{"type": "string", "format": "binary"}
		}
		if t == reflect.TypeFor[[]*multipart.FileHeader]() || t == reflect.TypeFor[[]multipart.FileHeader]() {
			return map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "binary"}}
		}
	}
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
		if t.Elem().Kind() == reflect.Uint8 && !strings.HasSuffix(mode, "/query") && !strings.HasSuffix(mode, "/header") {
			if strings.HasSuffix(mode, "/json") && !strings.HasPrefix(mode, "response/") {
				return nullable(map[string]any{"anyOf": []any{
					map[string]any{"type": "string", "contentEncoding": "base64"},
					map[string]any{"type": "array", "items": c.schema(t.Elem(), mode)},
				}})
			}
			return nullable(map[string]any{"type": "string", "contentEncoding": "base64"})
		}
		return nullable(map[string]any{"type": "array", "items": c.schema(t.Elem(), mode)})
	case reflect.Array:
		if strings.HasSuffix(mode, "/json") && !strings.HasPrefix(mode, "response/") {
			prefix := make([]any, t.Len())
			for i := range prefix {
				prefix[i] = c.schema(t.Elem(), mode)
			}
			result := map[string]any{"type": "array"}
			if len(prefix) > 0 {
				result["prefixItems"] = prefix
			}
			return result
		}
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
		hash := sha256.Sum256([]byte(typeIdentity(t) + ":" + mode))
		id := fmt.Sprintf("%s_%x", base, hash[:8])
		c.ids[key], c.schemas[id] = id, map[string]any{}
		c.schemas[id] = c.structure(t, mode)
		return reference(id)
	}
	c.diagnostic("TYPE_UNRESOLVED", t.String(), "This type has no supported JSON schema.")
	return map[string]any{}
}

func integerKind(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Uintptr }

func fileField(t reflect.Type) bool {
	return t == reflect.TypeFor[*multipart.FileHeader]() || t == reflect.TypeFor[[]*multipart.FileHeader]() || t == reflect.TypeFor[[]multipart.FileHeader]()
}
func nullable(schema any) any {
	return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
}
func reference(id string) any { return map[string]any{"$ref": "#/components/schemas/" + id} }

func customCodecType(t reflect.Type) bool {
	seen := map[reflect.Type]bool{}
	for t.Kind() == reflect.Pointer {
		if seen[t] {
			return true
		}
		seen[t] = true
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
	for _, source := range []string{"query", "header"} {
		if strings.HasSuffix(mode, "/"+source) {
			selected, err := input.Fields(t, source)
			if err != nil {
				c.diagnostic("INPUT_FIELDS_UNRESOLVED", t.String(), err.Error())
			}
			fields = nil
			for _, field := range selected {
				fields = append(fields, wireField{name: field.Name, index: field.Index, typeOf: field.Type})
			}
		}
	}
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
	name                       string
	index                      []int
	typeOf                     reflect.Type
	optional, quoted, indirect bool
}

func wireFields(root reflect.Type, form bool) []wireField {
	fields, _ := input.BodyFields(root, form)
	result := make([]wireField, 0, len(fields))
	for _, f := range fields {
		result = append(result, wireField{name: f.Name, index: f.Index, typeOf: f.Type, optional: f.Optional, quoted: f.Quoted, indirect: f.Indirect})
	}
	return result
}

func raw(value any) json.RawMessage { contents, _ := json.Marshal(value); return contents }

// 입력 DTO 상세 명세는 실제 선택한 검증기의 설명 계약이 있을 때만 제공합니다.
func (c *catalog) inputSchemaAvailable(t reflect.Type) bool {
	if _, ok := c.validator.(validation.SchemaDescriber); ok {
		return true
	}
	c.diagnostic("INPUT_SCHEMA_DESCRIPTION_UNAVAILABLE", t.String(), "Input DTO schemas require a configured validator with schema description support.")
	return false
}
