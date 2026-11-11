package multipart

import "io"

// UploadedFile은 기존 전체 업로드 조회 API의 파일 정보입니다.
//
// Deprecated: input.Body[T]의 form 필드에 mime/multipart.FileHeader를 사용하세요.
type UploadedFile struct {
	FieldName   string
	Filename    string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

// UploadedFiles는 요청의 모든 업로드 파일을 제공합니다.
//
// Deprecated: input.Body[T]의 form 필드에 *mime/multipart.FileHeader 또는
// []*mime/multipart.FileHeader를 선언하세요. 동적인 필드 전체 조회는 spine.Ctx.HTTPRequestContext().MultipartForm을 사용하세요.
type UploadedFiles struct {
	Files []UploadedFile
}
