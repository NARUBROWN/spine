package input

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"

	"github.com/NARUBROWN/spine/internal/httpfailure"
	"github.com/NARUBROWN/spine/internal/jsondecode"
)

const (
	defaultBodyBytes       = 32 << 20
	defaultMultipartMemory = 32 << 20
)

var (
	ErrBodyConsumed = errors.New("request body has already been bound")
	ErrBodyClosed   = errors.New("request body parsing has finished")
)

// BodyOptions는 공통 입력 구현의 내부 설정입니다. 공개 boot 설정과는 별개입니다.
type BodyOptions struct {
	// MaxBytes의 0은 현재 Echo 플랫폼과 같은 기본 한도, 음수는 제한 해제입니다.
	MaxBytes int64
	// MultipartMemory의 0은 현재 기본 메모리 예산입니다. 전체 본문 한도가 아닙니다.
	MultipartMemory int64
}

// Body는 요청 하나의 본문과 multipart 파싱 결과를 소유합니다.
// 요청 안에서 동기적으로 사용하며 원래 스트림을 닫거나 자동 재생하지 않습니다.
type Body struct {
	ctx         context.Context
	reader      *bufio.Reader
	source      *bodyContextReader
	contentType string
	memory      int64
	bound       bool
	empty       bool
	bindErr     error
	formParsed  bool
	form        *multipart.Form
	formErr     error
	closed      bool
	cleanupErr  error
}

// NewBody는 본문을 읽지 않습니다. 읽기는 Bind 또는 MultipartForm에서 시작합니다.
func NewBody(ctx context.Context, reader io.Reader, contentType string, options BodyOptions) (*Body, error) {
	if ctx == nil || reader == nil {
		return nil, fmt.Errorf("request body requires a context and a reader")
	}
	if options.MultipartMemory < 0 {
		return nil, fmt.Errorf("multipart memory budget cannot be negative")
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = defaultBodyBytes
	}
	if options.MultipartMemory == 0 {
		options.MultipartMemory = defaultMultipartMemory
	}
	tracked := &bodyContextReader{ctx: ctx, source: reader}
	var source io.Reader = tracked
	if options.MaxBytes > 0 {
		// NopCloser를 닫더라도 원래 스트림의 소유권은 호출자에게 남습니다.
		// MaxBytesReader는 길이를 모르는 본문도 초과를 EOF와 구분해 보고합니다.
		source = http.MaxBytesReader(nil, io.NopCloser(source), options.MaxBytes)
	}
	return &Body{ctx: ctx, reader: bufio.NewReader(source), source: tracked, contentType: contentType, memory: options.MultipartMemory}, nil
}

// Bind는 본문만 한 번 바인딩합니다. URL 쿼리나 라우팅 결과는 받지 않습니다.
// 폼 DTO와 MultipartForm은 같은 파싱 결과를 사용하며 실패한 파싱도 반복하지 않습니다.
func (b *Body) Bind(target any) (err error) {
	if err := b.active(); err != nil {
		return err
	}
	if b.bound {
		if b.bindErr != nil {
			return b.bindErr
		}
		return ErrBodyConsumed
	}
	b.bound = true
	defer func() { b.bindErr = err }()

	// 파일 리졸버가 먼저 파싱했다면 스트림의 EOF를 빈 본문으로 오인하지 않습니다.
	if b.formParsed {
		form, err := b.MultipartForm()
		if err != nil {
			return err
		}
		return b.decodeError(bindBodyForm(b.ctx, target, form.Value, form.File))
	}
	if _, err := b.reader.Peek(1); err != nil {
		if errors.Is(err, io.EOF) {
			b.empty = true
			return nil
		}
		return b.readError(err)
	}
	media, params, err := mime.ParseMediaType(b.contentType)
	if err != nil {
		if b.contentType == "" {
			return bodyHTTPError(http.StatusUnsupportedMediaType, err)
		}
		return b.decodeError(err)
	}
	if media == "multipart/form-data" {
		form, err := b.multipartForm(params)
		if err != nil {
			return err
		}
		return b.decodeError(bindBodyForm(b.ctx, target, form.Value, form.File))
	}
	if media != "application/json" && media != "application/xml" && media != "text/xml" && media != "application/x-www-form-urlencoded" {
		return bodyHTTPError(http.StatusUnsupportedMediaType, nil)
	}
	data, err := io.ReadAll(b.reader)
	if err != nil {
		return b.readError(err)
	}
	if err := b.ctx.Err(); err != nil {
		return err
	}
	switch media {
	case "application/json":
		err = jsondecode.Unmarshal(data, target)
	case "application/xml", "text/xml":
		err = decodeXML(data, target)
	case "application/x-www-form-urlencoded":
		var values url.Values
		values, err = url.ParseQuery(string(data))
		if err == nil {
			err = bindBodyForm(b.ctx, target, values, nil)
		}
	}
	return b.decodeError(err)
}

func decodeXML(data []byte, target any) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if root, ok := token.(xml.StartElement); ok {
			if err := decoder.DecodeElement(target, &root); err != nil {
				return err
			}
			break
		}
		if text, ok := token.(xml.CharData); ok && len(bytes.Trim(text, " \t\r\n")) != 0 {
			return fmt.Errorf("unexpected XML content before the root element")
		}
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.Comment:
			continue
		case xml.ProcInst:
			if value.Target != "xml" {
				continue
			}
		case xml.CharData:
			if len(bytes.Trim(value, " \t\r\n")) == 0 {
				continue
			}
		}
		return fmt.Errorf("unexpected XML content after the root element")
	}
}

// MultipartForm은 요청별 파싱 결과와 오류를 캐시합니다. 파일과 폼 리졸버의 순서는 무관합니다.
func (b *Body) MultipartForm() (*multipart.Form, error) {
	if err := b.active(); err != nil {
		return nil, err
	}
	if b.formParsed {
		return b.form, b.formErr
	}
	if b.bound && !b.empty {
		if b.bindErr != nil {
			return nil, b.bindErr
		}
		return nil, ErrBodyConsumed
	}
	media, params, err := mime.ParseMediaType(b.contentType)
	if err != nil {
		return nil, b.decodeError(err)
	}
	if media != "multipart/form-data" {
		return nil, b.decodeError(http.ErrNotMultipart)
	}
	return b.multipartForm(params)
}

func (b *Body) multipartForm(params map[string]string) (*multipart.Form, error) {
	b.formParsed = true
	boundary := params["boundary"]
	if boundary == "" {
		b.formErr = b.decodeError(http.ErrMissingBoundary)
		return nil, b.formErr
	}
	var err error
	b.form, err = multipart.NewReader(b.reader, boundary).ReadForm(b.memory)
	if err == nil {
		// 마지막 boundary 뒤의 데이터에도 전체 본문 한도를 적용합니다.
		_, err = io.Copy(io.Discard, b.reader)
	}
	if err != nil {
		// 표준 파서의 구문 오류와 스트림의 읽기 오류를 구분합니다.
		b.formErr = b.multipartError(err)
	} else {
		b.formErr = b.ctx.Err()
	}
	if b.formErr != nil && b.form != nil {
		b.formErr = errors.Join(b.formErr, b.form.RemoveAll())
		b.form = nil
	}
	return b.form, b.formErr
}

// Cleanup은 소유한 임시 파일을 한 번 정리합니다. 완료 훅이 끝난 뒤 호출합니다.
// 본문 스트림은 닫지 않으며 정리 후에는 파싱이나 파일 재조회를 허용하지 않습니다.
func (b *Body) Cleanup() error {
	if b.closed {
		return b.cleanupErr
	}
	b.closed = true
	if b.form != nil {
		b.cleanupErr = b.form.RemoveAll()
		b.form = nil
	}
	return b.cleanupErr
}

func (b *Body) active() error {
	if b.closed {
		return ErrBodyClosed
	}
	return b.ctx.Err()
}

func (b *Body) readError(err error) error {
	if ctxErr := b.ctx.Err(); ctxErr != nil {
		return errors.Join(ctxErr, err)
	}
	var limit *http.MaxBytesError
	if errors.As(err, &limit) || errors.Is(err, multipart.ErrMessageTooLarge) {
		return bodyHTTPError(http.StatusRequestEntityTooLarge, err)
	}
	return err
}

func (b *Body) decodeError(err error) error {
	if ctxErr := b.ctx.Err(); ctxErr != nil {
		return errors.Join(ctxErr, err)
	}
	if err == nil {
		return nil
	}
	return bodyHTTPError(http.StatusBadRequest, err)
}

func (b *Body) multipartError(err error) error {
	var limit *http.MaxBytesError
	if b.ctx.Err() != nil || errors.As(err, &limit) || errors.Is(err, multipart.ErrMessageTooLarge) {
		return b.readError(err)
	}
	if b.source.err != nil {
		return errors.Join(err, b.source.err)
	}
	return b.decodeError(err)
}

func bodyHTTPError(status int, cause error) error {
	return &httpfailure.HTTPError{Status: status, Message: http.StatusText(status), Cause: cause}
}

type bodyContextReader struct {
	ctx    context.Context
	source io.Reader
	err    error
}

func (r *bodyContextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.source.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, errors.Join(ctxErr, err)
	}
	return n, err
}
