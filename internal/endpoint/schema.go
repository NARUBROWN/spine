package endpoint

import (
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math/big"
	"mime/multipart"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	xmlInput := strings.HasSuffix(mode, "/xml")
	if strings.HasSuffix(mode, "/form") {
		if t == reflect.TypeFor[*multipart.FileHeader]() {
			return map[string]any{"type": "string", "format": "binary"}
		}
		if t == reflect.TypeFor[[]*multipart.FileHeader]() || t == reflect.TypeFor[[]multipart.FileHeader]() {
			return map[string]any{"type": "array", "items": map[string]any{"type": "string", "format": "binary"}}
		}
	}
	if !xmlInput && t == reflect.TypeFor[json.RawMessage]() {
		return map[string]any{}
	}
	if !xmlInput && t == reflect.TypeFor[json.Number]() {
		return map[string]any{"type": "number"}
	}
	if t == reflect.TypeFor[time.Time]() {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		if strings.HasSuffix(mode, "/path") || xmlInput {
			return c.schema(t.Elem(), mode)
		}
		return nullable(c.schema(t.Elem(), mode))
	}
	if xmlInput && (t.Implements(reflect.TypeFor[xml.Unmarshaler]()) || reflect.PointerTo(t).Implements(reflect.TypeFor[xml.Unmarshaler]()) ||
		t.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) || reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())) {
		c.diagnostic("XML_SERIALIZATION_UNRESOLVED", t.String(), "Custom XML or text decoding requires an explicit schema.")
		return map[string]any{}
	}
	if !xmlInput && customCodec(t) {
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
		result := map[string]any{"type": "integer", "minimum": json.Number(minimum.String()), "maximum": json.Number(maximum.String()), "x-spine-type-range": true, "x-spine-type-range-text": minimum.String() + " ~ " + maximum.String()}
		if t.Kind() >= reflect.Int && t.Kind() <= reflect.Int64 {
			if bits == 64 {
				result["format"] = "int64"
			} else if bits == 32 {
				result["format"] = "int32"
			}
		}
		return result
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Interface:
		c.diagnostic("DYNAMIC_TYPE_UNRESOLVED", t.String(), "An interface does not describe its runtime JSON representation.")
		return map[string]any{}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 && !strings.HasSuffix(mode, "/query") && !strings.HasSuffix(mode, "/header") {
			if xmlInput {
				return map[string]any{"type": "string"}
			}
			if strings.HasSuffix(mode, "/json") && !strings.HasPrefix(mode, "response/") {
				return nullable(map[string]any{"anyOf": []any{
					map[string]any{"type": "string", "contentEncoding": "base64"},
					map[string]any{"type": "array", "items": c.schema(t.Elem(), mode)},
				}})
			}
			return nullable(map[string]any{"type": "string", "contentEncoding": "base64"})
		}
		sequence := map[string]any{"type": "array", "items": c.schema(t.Elem(), mode)}
		if xmlInput {
			return sequence
		}
		return nullable(sequence)
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
		if xmlInput {
			c.diagnostic("XML_TYPE_UNRESOLVED", t.String(), "The default XML decoder does not bind map elements.")
			return map[string]any{}
		}
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

func (c *catalog) structure(t reflect.Type, mode string, planned ...[]input.Field) any {
	fields := wireFields(t, strings.HasSuffix(mode, "/form"))
	var xmlRoot map[string]any
	if strings.HasSuffix(mode, "/xml") {
		fields, xmlRoot = c.xmlFields(t)
	}
	for _, source := range []string{"path", "query", "header"} {
		if strings.HasSuffix(mode, "/"+source) {
			selected, err := input.Fields(t, source)
			if len(planned) > 0 {
				selected, err = planned[0], nil
			}
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
		if strings.HasSuffix(mode, "/path") || strings.HasPrefix(mode, "response/") && !field.optional && !field.indirect {
			required = append(required, field.name)
		}
		// Metadata describes the wire value and never changes runtime validation.
		source := t.FieldByIndex(field.index)
		annotation := map[string]any{}
		if field.xmlAttribute {
			annotation["xml"] = map[string]any{"attribute": true}
		}
		if description, ok := source.Tag.Lookup("description"); ok {
			annotation["description"] = description
		}
		if example, ok := source.Tag.Lookup("example"); ok {
			value, err := fieldExample(field.typeOf, field.quoted, example)
			if err != nil {
				c.diagnostic("FIELD_EXAMPLE_INVALID", t.String()+"."+field.name, "The example does not match the supported wire type.")
			} else {
				matches, known := true, true
				if exists && !field.quoted {
					matches, known = checkExampleConstraints(value, constraint.Schema)
				}
				if !matches {
					c.diagnostic("FIELD_EXAMPLE_INVALID", t.String()+"."+field.name, "The example conflicts with a described validation constraint.")
				} else {
					if !known {
						c.diagnostic("FIELD_EXAMPLE_VALIDATION_UNRESOLVED", t.String()+"."+field.name, "Some described constraints could not be checked without executing user code.")
					}
					annotation["examples"] = []any{value}
					annotation["example"] = value
				}
			}
		}
		if len(annotation) > 0 {
			annotated := map[string]any{"allOf": []any{schema}}
			for key, value := range annotation {
				annotated[key] = value
			}
			schema = annotated
		}
		properties[field.name] = schema
	}
	sort.Strings(required)
	required = slices.Compact(required)
	result := map[string]any{"type": "object", "properties": properties}
	if xmlRoot != nil {
		result["xml"] = xmlRoot
	}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

// checkExampleConstraints interprets supported schema keywords without invoking a validator.
// Unknown keywords return known=false rather than claiming full validation.
func checkExampleConstraints(value any, raw json.RawMessage) (bool, bool) {
	var schema any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&schema) != nil {
		return true, false
	}
	var check func(any) (bool, bool)
	equal := func(a, b any) bool {
		if x, ok := a.(json.Number); ok {
			if y, ok := b.(json.Number); ok {
				left, lok := new(big.Rat).SetString(x.String())
				right, rok := new(big.Rat).SetString(y.String())
				return lok && rok && left.Cmp(right) == 0
			}
		}
		return reflect.DeepEqual(a, b)
	}
	check = func(node any) (bool, bool) {
		m, ok := node.(map[string]any)
		if !ok {
			return true, false
		}
		known := true
		for key, raw := range m {
			switch key {
			case "allOf":
				clauses, ok := raw.([]any)
				if !ok {
					known = false
					continue
				}
				for _, clause := range clauses {
					matches, resolved := check(clause)
					if !matches {
						return false, true
					}
					known = known && resolved
				}
			case "anyOf", "oneOf":
				clauses, ok := raw.([]any)
				if !ok {
					known = false
					continue
				}
				matches, unknown := 0, false
				for _, clause := range clauses {
					match, resolved := check(clause)
					if !resolved {
						unknown = true
					}
					if match && resolved {
						matches++
					}
				}
				if !unknown && ((key == "anyOf" && matches == 0) || (key == "oneOf" && matches != 1)) {
					return false, true
				}
				if unknown {
					known = false
				}
			case "not":
				matches, resolved := check(raw)
				if !resolved {
					known = false
				} else if matches {
					return false, true
				}
			case "type":
				typ, ok := raw.(string)
				if !ok {
					known = false
					continue
				}
				switch typ {
				case "null":
					if value != nil {
						return false, true
					}
				case "string":
					if _, ok := value.(string); !ok {
						return false, true
					}
				case "number", "integer":
					if _, ok := value.(json.Number); !ok {
						return false, true
					}
				case "boolean":
					if _, ok := value.(bool); !ok {
						return false, true
					}
				default:
					known = false
				}
			case "const":
				if !equal(value, raw) {
					return false, true
				}
			case "enum":
				values, ok := raw.([]any)
				if !ok {
					known = false
					continue
				}
				found := false
				for _, candidate := range values {
					if equal(value, candidate) {
						found = true
						break
					}
				}
				if !found {
					return false, true
				}
			case "minLength", "maxLength":
				text, ok := value.(string)
				if !ok {
					continue
				}
				n, ok := raw.(json.Number)
				if !ok {
					known = false
					continue
				}
				limit, err := strconv.Atoi(n.String())
				if err != nil {
					known = false
					continue
				}
				length := utf8.RuneCountInString(text)
				if (key == "minLength" && length < limit) || (key == "maxLength" && length > limit) {
					return false, true
				}
			case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
				number, ok := value.(json.Number)
				if !ok {
					continue
				}
				bound, ok := raw.(json.Number)
				if !ok {
					known = false
					continue
				}
				actual, aok := new(big.Rat).SetString(number.String())
				limit, lok := new(big.Rat).SetString(bound.String())
				if !aok || !lok {
					known = false
					continue
				}
				cmp := actual.Cmp(limit)
				if (key == "minimum" && cmp < 0) || (key == "maximum" && cmp > 0) || (key == "exclusiveMinimum" && cmp <= 0) || (key == "exclusiveMaximum" && cmp >= 0) {
					return false, true
				}
			case "pattern":
				// JSON Schema uses ECMA patterns, which Go cannot safely interpret in general.
				known = false
			case "format":
				text, ok := value.(string)
				if !ok {
					continue
				}
				format, ok := raw.(string)
				if !ok {
					known = false
					continue
				}
				switch format {
				case "email":
					// Validators differ on quoted and internationalized addresses.
					separator := strings.LastIndexByte(text, '@')
					if separator <= 0 || separator == len(text)-1 || strings.ContainsAny(text, "\r\n") {
						return false, true
					}
					known = false
				case "uuid":
					if !regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(text) {
						return false, true
					}
					// A schema format does not reveal narrower validator variants such as uuid4.
					known = false
				case "uri":
					parsed, err := url.Parse(text)
					if err != nil || parsed.Scheme == "" {
						return false, true
					}
					known = false
				default:
					known = false
				}
			case "title", "description", "default", "examples", "example", "$comment":
			default:
				known = false
			}
		}
		return true, known
	}
	return check(schema)
}
func exampleMatchesConstraints(value any, raw json.RawMessage) bool {
	matches, _ := checkExampleConstraints(value, raw)
	return matches
}

func fieldExample(t reflect.Type, quoted bool, value string) (any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeFor[time.Time]() {
		if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
			return nil, err
		}
		return value, nil
	}
	if customCodec(t) {
		return nil, fmt.Errorf("custom codec")
	}
	if quoted {
		if t.Kind() == reflect.String {
			return nil, fmt.Errorf("quoted string example unsupported")
		}
		_, err := fieldExample(t, false, value)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
	switch t.Kind() {
	case reflect.String:
		return value, nil
	case reflect.Bool:
		return strconv.ParseBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if _, err := strconv.ParseInt(value, 10, t.Bits()); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(value)) {
			return nil, fmt.Errorf("invalid JSON integer")
		}
		return json.Number(value), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if _, err := strconv.ParseUint(value, 10, t.Bits()); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(value)) {
			return nil, fmt.Errorf("invalid JSON integer")
		}
		return json.Number(value), nil
	case reflect.Float32, reflect.Float64:
		if _, err := strconv.ParseFloat(value, t.Bits()); err != nil {
			return nil, err
		}
		if !json.Valid([]byte(value)) {
			return nil, fmt.Errorf("invalid JSON number")
		}
		return json.Number(value), nil
	default:
		return nil, fmt.Errorf("unsupported example type")
	}
}

type wireField struct {
	name                       string
	index                      []int
	typeOf                     reflect.Type
	optional, quoted, indirect bool
	xmlAttribute               bool
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
