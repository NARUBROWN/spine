package multipart

import "io"

type UploadedFile struct {
	FieldName   string
	Filename    string
	ContentType string
	Size        int64
	Open        func() (io.ReadCloser, error)
}

type UploadedFiles struct {
	Files []UploadedFile
}
