package core

import "context"

type ExecutionContext interface {
	// Pipeline / Router 관련 메서드
	Context() context.Context
	Method() string
	Path() string
	Params() map[string]string
	Header(name string) string
	PathKeys() []string
	Queries() map[string][]string
	Set(key string, value any)
	Get(key string) (any, bool)
}

type RequestContext interface {
	// Resolver 관련 메서드

	// 개별 접근
	Param(name string) string
	Query(name string) string

	// 전체 뷰 접근
	Params() map[string]string
	Queries() map[string][]string

	// body
	Bind(out any) error
}
