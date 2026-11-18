package multipart

import "io"

// UploadedFile describes a file in the legacy upload lookup API.
//
// Deprecated: Use mime/multipart.FileHeader in a form field of input.Body[T].
type UploadedFile struct {
	FieldName   string
	Filename    string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

// UploadedFiles provides all uploaded files in a request.
//
// Deprecated: Declare *mime/multipart.FileHeader or []*mime/multipart.FileHeader
// in a form field of input.Body[T]. For dynamic field names, obtain the request
// view with spine.Ctx.HTTPRequestContext and call its MultipartForm method.
type UploadedFiles struct {
	Files []UploadedFile
}
