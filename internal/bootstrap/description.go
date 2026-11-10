package bootstrap

import (
	"fmt"
	"mime"
	"strings"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/endpoint"
	"github.com/NARUBROWN/spine/internal/openapi/resourcepath"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// DescribeHTTP는 준비 과정과 달리 factory, 생성자와 외부 자원을 초기화하지 않습니다.
func DescribeHTTP(config Config) (description.HTTP, error) {
	config = snapshotConfig(config)
	if err := boot.ValidationError(append(controllerProviderIssues(config), componentValidationIssues(config)...)...); err != nil {
		return description.HTTP{}, err
	}
	if config.HTTP != nil && !config.RoutesCollected && len(controllerTypes(config.controllerCandidates())) > 0 {
		return description.HTTP{}, ErrRoutesNotCollected
	}
	if config.ValidatorConfigured || config.Validator != nil {
		if err := validation.CheckValidator(config.Validator); err != nil {
			return description.HTTP{}, err
		}
	}
	return endpoint.Describe(config.Routes, config.HTTP, config.Validator)
}

func openAPICollisionIssues(config Config, resources []boot.OpenAPIResource) []boot.ConfigIssue {
	if config.HTTP == nil || !config.HTTP.OpenAPI.Enabled {
		return nil
	}
	var issues []boot.ConfigIssue
	resources = append([]boot.OpenAPIResource{{Path: config.HTTP.OpenAPI.DocumentPath()}}, resources...)
	prefix := strings.TrimSuffix(config.HTTP.GlobalPrefix, "/")
	for _, resource := range resources {
		if !resourcepath.Valid(resource.Path) {
			continue
		}
		collision := func() {
			issues = append(issues, boot.ConfigIssue{Path: "HTTP.OpenAPI", Code: "OPENAPI_PATH_CONFLICT", Message: fmt.Sprintf("OpenAPI resource %q conflicts with an application route.", resource.Path), Hint: "Choose another document or UI path, or disable the resource."})
		}
		for _, route := range config.Routes {
			fullPath, err := joinPath(prefix, route.Path)
			if err == nil && (route.Method == "GET" || route.Method == "HEAD") && assertNoAmbiguousRoute(route.Method, resource.Path, []string{fullPath}) != nil {
				collision()
				break
			}
		}
		if config.WebSocketRegistry != nil {
			for _, route := range config.WebSocketRegistry.Registrations() {
				if assertNoAmbiguousRoute("GET", resource.Path, []string{route.Path}) != nil {
					collision()
					break
				}
			}
		}
	}
	return issues
}

func openAPIUIResources(options boot.OpenAPIOptions) ([]boot.OpenAPIResource, []boot.ConfigIssue) {
	if !options.Enabled {
		return nil, nil
	}
	var resources []boot.OpenAPIResource
	var issues []boot.ConfigIssue
	seen := map[string]bool{options.DocumentPath(): true}
	for i, provider := range options.UI {
		location := fmt.Sprintf("HTTP.OpenAPI.UI[%d]", i)
		if provider == nil {
			issues = append(issues, boot.ConfigIssue{Path: location, Code: "OPENAPI_UI_INVALID", Message: "OpenAPI UI provider is nil.", Hint: "Remove the nil provider."})
			continue
		}
		provided, err := provider(options.DocumentPath())
		if err != nil {
			issues = append(issues, boot.ConfigIssue{Path: location, Code: "OPENAPI_UI_INVALID", Message: err.Error(), Hint: "Check the UI path and options."})
			continue
		}
		for j, resource := range provided {
			resourceLocation := fmt.Sprintf("%s.Resources[%d]", location, j)
			_, _, contentTypeErr := mime.ParseMediaType(resource.ContentType)
			if !resourcepath.Valid(resource.Path) || contentTypeErr != nil || strings.ContainsAny(resource.ContentType, "\r\n") {
				issues = append(issues, boot.ConfigIssue{Path: resourceLocation, Code: "OPENAPI_UI_RESOURCE_INVALID", Message: "UI resources require static absolute paths and valid content types.", Hint: "Check the UI resource configuration."})
				continue
			}
			if seen[resource.Path] {
				issues = append(issues, boot.ConfigIssue{Path: resourceLocation, Code: "OPENAPI_PATH_CONFLICT", Message: fmt.Sprintf("OpenAPI resources share path %q.", resource.Path), Hint: "Choose distinct document and UI paths."})
				continue
			}
			seen[resource.Path] = true
			resources = append(resources, resource)
		}
	}
	return resources, issues
}
