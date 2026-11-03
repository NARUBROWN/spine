// Package httpplan은 HTTP 실행과 설명이 공유하는 기본 처리 목록을 제공합니다.
package httpplan

import (
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/resolver"
)

func Resolvers(validator validation.Validator) []resolver.ArgumentResolver {
	return []resolver.ArgumentResolver{
		&resolver.StdContextResolver{}, &resolver.ControllerContextResolver{},
		&resolver.PathIntResolver{},
		&resolver.PathStringResolver{}, &resolver.PathBooleanResolver{},
		&resolver.PaginationResolver{}, &resolver.InputDTOResolver{Validator: validator},
		&resolver.DTOResolver{Validator: validator}, &resolver.FormDTOResolver{Validator: validator},
		&resolver.UploadedFilesResolver{},
	}
}

func Returns() []handler.ReturnValueHandler {
	return []handler.ReturnValueHandler{
		&handler.RedirectReturnValueHandler{}, &handler.BinaryReturnHandler{},
		&handler.StringReturnHandler{}, &handler.JSONReturnHandler{}, &handler.ErrorReturnHandler{},
	}
}
