// Package endpoint는 HTTP 엔드포인트의 입력과 반환 규칙, 사전 검사와 명세 생성을 제공합니다.
package endpoint

import (
	validation "github.com/NARUBROWN/spine/contract/validator"
	"github.com/NARUBROWN/spine/internal/handler"
	"github.com/NARUBROWN/spine/internal/resolver"
)

func Resolvers(validator validation.Validator) []resolver.ArgumentResolver {
	return []resolver.ArgumentResolver{
		&resolver.StdContextResolver{}, &resolver.ControllerContextResolver{},
		&resolver.HeaderResolver{}, &resolver.PathIntResolver{},
		&resolver.PathStringResolver{}, &resolver.PathBooleanResolver{},
		&resolver.PaginationResolver{}, &resolver.QueryValuesResolver{},
		&resolver.ScalarPathResolver{}, &resolver.InputDTOResolver{Validator: validator},
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
