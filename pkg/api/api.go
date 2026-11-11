// Package api는 컨트롤러 안에서 HTTP 경로와 핸들러를 등록하는 계약입니다.
package api

import "github.com/NARUBROWN/spine/internal/apiregistry"

// Registry는 앱 준비 중에만 사용하는 경로 등록기입니다.
type Registry = apiregistry.Registry

// Controller는 기존 DI 생성자로 등록하는 HTTP 컨트롤러의 최소 계약입니다.
// 추가 메서드와 의존성 인자는 제한하지 않습니다.
// constructor.Controller 등록의 공통 Constructor 생성 계약은 별도로 사전 검사합니다.
type Controller interface {
	RegisterRoutes(*Registry)
}

// Group은 공통 경로와 설정을 보유합니다.
type Group = apiregistry.Group

// Option은 Registry와 Group에 전달하는 경로 설정입니다. Tags, Summary, Description 또는 Interceptors로 만듭니다.
type Option = apiregistry.Option

var Tags = apiregistry.Tags
var Summary = apiregistry.Summary
var Description = apiregistry.Description
var Interceptors = apiregistry.Interceptors
