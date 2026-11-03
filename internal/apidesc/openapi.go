package apidesc

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/NARUBROWN/spine/contract/description"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// OpenAPI는 이미 설명한 계약만 직렬화하며 앱이나 검증기를 다시 실행하지 않습니다.
func OpenAPI(contract description.HTTP, options boot.OpenAPIOptions) (json.RawMessage, error) {
	title, version := options.Title, options.Version
	if title == "" {
		title = "Spine API"
	}
	if version == "" {
		version = "unversioned"
	}
	paths := map[string]any{}
	diagnostics := append([]description.Diagnostic{}, contract.Diagnostics...)
	for _, route := range contract.Routes {
		method := strings.ToLower(route.Method)
		switch method {
		case "get", "put", "post", "delete", "options", "head", "patch", "trace":
		default:
			diagnostics = append(diagnostics, description.Diagnostic{Code: "HTTP_METHOD_UNRESOLVED", Target: route.Handler, Message: "This HTTP method is outside OpenAPI 3.1 operations."})
			continue
		}
		segments := strings.Split(route.Path, "/")
		validPath := true
		for i, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				if len(segment) == 1 || strings.ContainsAny(segment[1:], "{}") {
					validPath = false
					break
				}
				segments[i] = "{" + segment[1:] + "}"
			} else {
				segments[i] = url.PathEscape(segment)
			}
		}
		if !validPath {
			diagnostics = append(diagnostics, description.Diagnostic{Code: "PATH_TEMPLATE_UNRESOLVED", Target: route.Handler, Message: "The registered path has no valid OpenAPI template."})
			continue
		}
		path := strings.Join(segments, "/")
		item, _ := paths[path].(map[string]any)
		if item == nil {
			item = map[string]any{}
			paths[path] = item
		}
		id := sha256.Sum256([]byte(route.Method + " " + route.Path))
		op := map[string]any{"operationId": fmt.Sprintf("%s_%x", method, id[:8])}
		parameters := []any{}
		body := map[string]any{}
		validation := false
		for _, input := range route.Inputs {
			switch input.Binding {
			case "path", "query":
				if input.Name != "" {
					parameters = append(parameters, map[string]any{"name": input.Name, "in": input.Binding, "required": input.Required, "schema": input.Schema})
				}
			case "body":
				if len(body) > 0 {
					diagnostics = append(diagnostics, description.Diagnostic{Code: "MULTIPLE_BODY_PARAMETERS", Target: route.Handler, Message: "Multiple DTO parameters cannot be represented as independent HTTP bodies."})
					continue
				}
				for media, schema := range input.Content {
					body[media] = map[string]any{"schema": schema}
				}
				validation = validation || input.Validation
			}
		}
		if len(parameters) > 0 {
			op["parameters"] = parameters
		}
		if len(body) > 0 {
			op["requestBody"] = map[string]any{"content": body}
		}
		content := map[string]any{}
		for _, output := range route.Returns {
			for media, schema := range output.Content {
				if previous, ok := content[media].(map[string]any); ok {
					content[media] = map[string]any{"schema": map[string]any{"anyOf": []any{previous["schema"], schema}}}
				} else {
					content[media] = map[string]any{"schema": schema}
				}
			}
		}
		response := map[string]any{"description": "Status codes are selected at runtime; content includes declared returns and framework error responses."}
		if jsonContent, ok := content["application/json"].(map[string]any); ok {
			jsonContent["schema"] = map[string]any{"anyOf": []any{jsonContent["schema"], errorSchema(false)}}
		} else if _, wildcard := content["*/*"]; !wildcard {
			content["application/json"] = map[string]any{"schema": errorSchema(false)}
		}

		if len(content) > 0 {
			response["content"] = content
		}
		responses := map[string]any{"default": response}
		if len(body) > 0 || len(parameters) > 0 {
			responses["400"] = map[string]any{"description": "Request binding or DTO validation failed.", "content": map[string]any{"application/json": map[string]any{"schema": errorSchema(validation)}}}
		}
		op["responses"] = responses
		item[method] = op
	}
	document := map[string]any{"openapi": "3.1.2", "info": map[string]any{"title": title, "version": version}, "paths": paths, "components": map[string]any{"schemas": contract.Schemas}}
	if len(diagnostics) > 0 {
		document["x-spine-diagnostics"] = diagnostics
	}
	return json.Marshal(document)
}

func errorSchema(validation bool) any {
	properties := map[string]any{"message": map[string]any{"type": "string"}}
	if validation {
		properties["fields"] = map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"field", "rule"}, "properties": map[string]any{"field": map[string]any{"type": "string"}, "rule": map[string]any{"type": "string"}, "param": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}}}}
	}
	return map[string]any{"type": "object", "required": []string{"message"}, "properties": properties}
}
