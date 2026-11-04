package endpoint

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/handler"
	inputbinding "github.com/NARUBROWN/spine/internal/input"
	"github.com/NARUBROWN/spine/internal/resolver"
	"github.com/NARUBROWN/spine/internal/router"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// Describe는 타입과 등록만 읽으며 리졸버의 Resolve나 반환 처리의 Handle을 실행하지 않습니다.
func Describe(routes []router.RouteSpec, options *boot.HTTPOptions, validator validation.Validator) (description.HTTP, error) {
	result := description.HTTP{ContractVersion: description.ContractVersion, Enabled: options != nil, Routes: []description.Route{}, Schemas: map[string]json.RawMessage{}, Diagnostics: []description.Diagnostic{}}
	if options == nil {
		return result, nil
	}
	result.PathsOnly = validator == nil
	if err := boot.ValidationError(options.ValidateIssues("HTTP")...); err != nil {
		return result, err
	}
	prefix := strings.TrimSuffix(options.GlobalPrefix, "/")
	if prefix != "" && (!strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, ":*")) {
		return result, fmt.Errorf("invalid HTTP global prefix")
	}
	c := newCatalog(validator)
	if result.PathsOnly {
		c.diagnostic("VALIDATOR_NOT_CONFIGURED", "HTTP", "Only registered paths and methods are described because no validator is configured.")
	}
	seen := map[string]bool{}
	var inputIssues []boot.ConfigIssue
	for index, spec := range routes {
		meta, err := router.NewHandlerMeta(spec.Handler)
		if err != nil {
			return result, err
		}
		inputIssues = append(inputIssues, DTOParameterIssues(meta.Method, fmt.Sprintf("Routes[%d]", index), spec.Path)...)
	}
	if err := boot.ValidationError(inputIssues...); err != nil {
		return result, err
	}
	for index, spec := range routes {
		meta, _ := router.NewHandlerMeta(spec.Handler)
		inputIssues = append(inputIssues, ValidationParameterIssues(meta.Method, fmt.Sprintf("Routes[%d]", index), validator)...)
	}
	if err := boot.ValidationError(inputIssues...); err != nil {
		return result, err
	}
	for _, spec := range routes {
		meta, err := router.NewHandlerMeta(spec.Handler)
		if err != nil {
			return result, err
		}
		fullPath, err := Path(prefix, spec.Path)
		if err != nil {
			return result, err
		}
		shape := strings.Split(fullPath, "/")
		for i, segment := range shape {
			if strings.HasPrefix(segment, ":") {
				shape[i] = "{}"
			}
		}
		key := spec.Method + " " + strings.Join(shape, "/")
		if seen[key] {
			return result, fmt.Errorf("duplicate HTTP route: %s", key)
		}
		seen[key] = true
		route := description.Route{Method: spec.Method, Path: fullPath, Handler: meta.ControllerType.String() + "." + meta.Method.Name, Inputs: []description.Input{}, Returns: []description.Return{}}
		if result.PathsOnly {
			result.Routes = append(result.Routes, route)
			continue
		}
		var pathKeys []string
		for _, segment := range strings.Split(fullPath, "/") {
			if strings.HasPrefix(segment, ":") {
				pathKeys = append(pathKeys, segment[1:])
			}
		}
		for _, parameter := range resolver.Parameters(meta.Method, pathKeys) {
			input := description.Input{Index: parameter.Index, Type: parameter.Type.String()}
			matched := false
			for _, candidate := range Resolvers(nil) {
				if !candidate.Supports(parameter) {
					continue
				}
				matched = true
				switch candidate.(type) {
				case *resolver.InputDTOResolver:
					payload, source, _ := inputbinding.DTO(parameter.Type)
					if _, err := inputbinding.Fields(payload, source); err != nil {
						return result, err
					}
					if !c.inputSchemaAvailable(payload) {
						input.Binding, input.Validation = source, validator != nil
						break
					}
					structure := c.structure(payload, "request/"+source).(map[string]any)
					required := map[string]bool{}
					if names, ok := structure["required"].([]string); ok {
						for _, name := range names {
							required[name] = true
						}
					}
					for name, schema := range structure["properties"].(map[string]any) {
						route.Inputs = append(route.Inputs, description.Input{Index: parameter.Index, Binding: source, Name: name, Type: payload.String(), Required: required[name], Schema: raw(schema), Validation: true})
					}
				case *resolver.StdContextResolver, *resolver.ControllerContextResolver:
					input.Binding = "context"
				case *resolver.PathIntResolver, *resolver.PathStringResolver, *resolver.PathBooleanResolver:
					input.Binding, input.Name, input.Required = "path", parameter.PathKey, true
					if input.Name == "" {
						c.diagnostic("PATH_KEY_UNRESOLVED", route.Handler, "A path wrapper has no registered path key.")
					}
					kind := "string"
					if _, ok := candidate.(*resolver.PathIntResolver); ok {
						kind = "integer"
					}
					if _, ok := candidate.(*resolver.PathBooleanResolver); ok {
						kind = "boolean"
					}
					schema := map[string]any{"type": kind}
					if kind == "integer" {
						schema["format"] = "int64"
					}
					input.Schema = raw(schema)
				case *resolver.PaginationResolver:
					for _, pagination := range []struct {
						name  string
						value int
					}{{"page", 1}, {"size", 20}} {
						route.Inputs = append(route.Inputs, description.Input{Index: parameter.Index, Binding: "query", Name: pagination.name, Type: parameter.Type.String(), Schema: raw(map[string]any{"type": "integer", "default": pagination.value})})
					}
					continue
				case *resolver.DTOResolver, *resolver.FormDTOResolver:
					input.Binding, input.Validation = "body", validator != nil
					if !c.inputSchemaAvailable(parameter.Type.Elem()) {
						break
					}
					input.Content = map[string]json.RawMessage{"application/json": raw(c.schema(parameter.Type.Elem(), "request/json"))}
					if id, ok := c.ids[schemaKey{parameter.Type.Elem(), "request/json"}]; ok {
						if schema, ok := c.schemas[id].(map[string]any); ok {
							if names, ok := schema["required"].([]string); ok {
								input.Required = len(names) > 0
							}
						}
					}
					if _, ok := candidate.(*resolver.FormDTOResolver); ok {
						form := raw(c.schema(parameter.Type.Elem(), "request/form"))
						input.Content["application/x-www-form-urlencoded"], input.Content["multipart/form-data"] = form, append(json.RawMessage(nil), form...)
						for _, field := range wireFields(parameter.Type.Elem(), true) {
							if fileField(field.typeOf) {
								input.Content = map[string]json.RawMessage{"multipart/form-data": form}
								break
							}
						}
					}
				case *resolver.UploadedFilesResolver:
					input.Binding = "unresolved"
					c.diagnostic("UPLOAD_FIELDS_UNRESOLVED", route.Handler, "UploadedFiles does not declare multipart field names.")
				}
				break
			}
			if !matched {
				input.Binding = "unresolved"
				c.diagnostic("PARAMETER_UNSUPPORTED", route.Handler, "No default argument resolver supports "+input.Type)
			}
			// pagination은 쿼리 두 개를 위에서 추가했습니다.
			if input.Binding != "" {
				route.Inputs = append(route.Inputs, input)
			}
		}
		for _, key := range pathKeys {
			found := false
			for _, input := range route.Inputs {
				if input.Binding == "path" && input.Name == key {
					found = true
				}
			}
			if !found {
				route.Inputs = append(route.Inputs, description.Input{Index: -1, Binding: "path", Name: key, Required: true, Schema: raw(map[string]any{})})
				c.diagnostic("PATH_TYPE_UNRESOLVED", route.Handler, "A registered path key has no typed path argument.")
			}
		}
		for i := 0; i < meta.Method.Type.NumOut(); i++ {
			t := meta.Method.Type.Out(i)
			output := description.Return{Type: t.String(), Kind: "unresolved"}
			for _, candidate := range Returns() {
				if !candidate.Supports(t) {
					continue
				}
				switch candidate.(type) {
				case *handler.ErrorReturnHandler:
					output.Kind = "error"
				case *handler.JSONReturnHandler, *handler.StringReturnHandler:
					if t.Kind() == reflect.Pointer {
						t = t.Elem()
					}
					body, _ := t.FieldByName("Body")
					media := "application/json"
					output.Kind = "json"
					if _, ok := candidate.(*handler.StringReturnHandler); ok {
						media, output.Kind = "text/plain", "string"
					}
					output.Content = map[string]json.RawMessage{media: raw(c.schema(body.Type, "response/json"))}
				case *handler.BinaryReturnHandler:
					output.Kind = "binary"
					output.Content = map[string]json.RawMessage{"*/*": raw(map[string]any{})}
					c.diagnostic("BINARY_CONTENT_TYPE_DYNAMIC", route.Handler, "Binary.ContentType is selected at runtime.")
				case *handler.RedirectReturnValueHandler:
					output.Kind = "redirect"
				}
				break
			}
			if output.Kind == "unresolved" {
				c.diagnostic("RETURN_UNSUPPORTED", route.Handler, "No default return handler supports "+output.Type)
			}
			route.Returns = append(route.Returns, output)
		}
		c.diagnostic("RESPONSE_STATUS_DYNAMIC", route.Handler, "ResponseOptions and application errors select status codes at runtime.")
		sort.SliceStable(route.Inputs, func(i, j int) bool {
			a, b := route.Inputs[i], route.Inputs[j]
			if a.Index != b.Index {
				return a.Index < b.Index
			}
			return a.Name < b.Name
		})
		result.Routes = append(result.Routes, route)
	}
	for name, schema := range c.schemas {
		result.Schemas[name] = raw(schema)
	}
	result.Diagnostics = append(result.Diagnostics, c.diagnostics...)
	sort.Slice(result.Routes, func(i, j int) bool {
		a, b := result.Routes[i], result.Routes[j]
		return a.Path+"\x00"+a.Method < b.Path+"\x00"+b.Method
	})
	sort.Slice(result.Diagnostics, func(i, j int) bool {
		a, b := result.Diagnostics[i], result.Diagnostics[j]
		return a.Target+"\x00"+a.Code+"\x00"+a.Message < b.Target+"\x00"+b.Code+"\x00"+b.Message
	})
	return result, nil
}
