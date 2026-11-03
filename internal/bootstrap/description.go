package bootstrap

import (
	"strings"

	"github.com/NARUBROWN/spine/contract/description"
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/apidesc"
	"github.com/NARUBROWN/spine/pkg/boot"
)

// DescribeHTTP는 준비 과정과 달리 factory, 생성자와 외부 자원을 초기화하지 않습니다.
func DescribeHTTP(config Config) (description.HTTP, error) {
	config = snapshotConfig(config)
	if config.ValidatorConfigured || config.Validator != nil {
		if err := validation.CheckValidator(config.Validator); err != nil {
			return description.HTTP{}, err
		}
	}
	return apidesc.Describe(config.Routes, config.HTTP, config.Validator)
}

func openAPICollisionIssues(config Config) []boot.ConfigIssue {
	if config.HTTP == nil || config.HTTP.OpenAPI.Disabled {
		return nil
	}
	documentPath := config.HTTP.OpenAPI.DocumentPath()
	prefix := strings.TrimSuffix(config.HTTP.GlobalPrefix, "/")
	collision := func() []boot.ConfigIssue {
		return []boot.ConfigIssue{{Path: "HTTP.OpenAPI.Path", Code: "OPENAPI_PATH_CONFLICT", Message: "OpenAPI path conflicts with an application route.", Hint: "Choose another OpenAPI path or disable the resource."}}
	}
	for _, route := range config.Routes {
		fullPath, err := joinPath(prefix, route.Path)
		if err == nil && strings.Trim(fullPath, "/") == strings.Trim(documentPath, "/") && (route.Method == "GET" || route.Method == "HEAD") {
			return collision()
		}
	}
	if config.WebSocketRegistry != nil {
		for _, route := range config.WebSocketRegistry.Registrations() {
			if strings.Trim(route.Path, "/") == strings.Trim(documentPath, "/") {
				return collision()
			}
		}
	}
	return nil
}
