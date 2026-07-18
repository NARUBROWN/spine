package resolver

import (
	"fmt"
	"io"
	"reflect"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/pkg/multipart"
)

type UploadedFilesResolver struct{}

func (r *UploadedFilesResolver) Supports(parameterMeta ParameterMeta) bool {
	return parameterMeta.Type == reflect.TypeFor[multipart.UploadedFiles]()
}

func (r *UploadedFilesResolver) Resolve(ctx core.ExecutionContext, parameterMeta ParameterMeta) (any, error) {
	httpCtx, ok := ctx.(core.HttpRequestContext)
	if !ok {
		return nil, fmt.Errorf("context is not an HTTP request context")
	}

	form, err := httpCtx.MultipartForm()
	if err != nil {
		return nil, err
	}

	var files []multipart.UploadedFile

	for fieldName, headers := range form.File {
		for _, h := range headers {
			header := h

			files = append(files, multipart.UploadedFile{
				FieldName:   fieldName,
				Filename:    header.Filename,
				ContentType: header.Header.Get("Content-Type"),
				Size:        header.Size,
				Open: func() (io.ReadCloser, error) {
					return header.Open()
				},
			})
		}
	}

	return multipart.UploadedFiles{Files: files}, nil
}
