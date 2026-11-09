package input

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/jsonx"
)

type bodyProbeReader struct {
	reader io.Reader
	reads  int
	closes int
}

func (r *bodyProbeReader) Read(p []byte) (int, error) { r.reads++; return r.reader.Read(p) }
func (r *bodyProbeReader) Close() error               { r.closes++; return nil }

func newBodyForTest(t *testing.T, ctx context.Context, reader io.Reader, media string, options BodyOptions) *Body {
	t.Helper()
	body, err := NewBody(ctx, reader, media, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := body.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	return body
}

func assertBodyStatus(t *testing.T, err error, status int) {
	t.Helper()
	if status == 0 {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var failure *httperr.HTTPError
	if !errors.As(err, &failure) || failure.Status != status || failure.Message != http.StatusText(status) {
		t.Fatalf("입력 오류 = %v, 기대 상태 = %d", err, status)
	}
}

func TestRequestBodyIsLazyAndNeverClosesTheSource(t *testing.T) {
	source := &bodyProbeReader{reader: strings.NewReader(`{"count":2}`)}
	body := newBodyForTest(t, t.Context(), source, "application/json", BodyOptions{})
	if source.reads != 0 {
		t.Fatal("요청 컨텍스트 생성만으로 본문을 읽으면 안 됩니다")
	}
	var dto struct{ Count int }
	if err := body.Bind(&dto); err != nil || dto.Count != 2 {
		t.Fatalf("바인딩 = %+v, 오류 = %v", dto, err)
	}
	reads := source.reads
	if !errors.Is(body.Bind(&dto), ErrBodyConsumed) || source.reads != reads {
		t.Fatal("본문을 자동 재생하거나 다시 읽으면 안 됩니다")
	}
	if err := body.Cleanup(); err != nil || source.closes != 0 {
		t.Fatalf("정리는 스트림을 닫으면 안 됩니다: %v", err)
	}
	if !errors.Is(body.Bind(&dto), ErrBodyClosed) {
		t.Fatal("정리가 끝난 요청을 다시 바인딩하면 안 됩니다")
	}
	if _, err := body.MultipartForm(); !errors.Is(err, ErrBodyClosed) {
		t.Fatal("정리가 끝난 요청에서 파일을 다시 조회하면 안 됩니다")
	}
}

func TestRequestBodyFormatsAndErrorStatus(t *testing.T) {
	for _, tc := range []struct {
		name, media, data string
		status, count     int
	}{
		{"json", "Application/JSON; charset=utf-8", `{"count":2}`, 0, 2},
		{"xml", "application/xml", `<item><count>3</count></item>`, 0, 3},
		{"text_xml", "text/xml", `<item><count>4</count></item>`, 0, 4},
		{"missing_field", "application/json", `{}`, 0, 0},
		{"unknown_field", "application/json", `{"count":2,"extra":true}`, 0, 2},
		{"malformed_json", "application/json", `{`, 400, 0},
		{"scalar_null", "application/json", `{"count":null}`, 400, 0},
		{"multiple_json_values", "application/json", `{} {}`, 400, 0},
		{"malformed_xml", "application/xml", `<item>`, 400, 0},
		{"unsupported", "text/plain", `payload`, 415, 0},
		{"missing_type", "", `{}`, 415, 0},
		{"invalid_type", "application/json; =bad", `{}`, 400, 0},
		{"empty_unsupported", "text/plain", "", 0, 0},
		{"empty_missing_type", "", "", 0, 0},
		{"whitespace_is_not_empty", "application/json", " ", 400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newBodyForTest(t, t.Context(), strings.NewReader(tc.data), tc.media, BodyOptions{})
			var dto struct {
				Count int `json:"count" xml:"count"`
			}
			err := body.Bind(&dto)
			assertBodyStatus(t, err, tc.status)
			if tc.status == 0 && dto.Count != tc.count {
				t.Fatalf("바인딩 값 = %d, 기대값 = %d", dto.Count, tc.count)
			}
			if tc.name == "scalar_null" {
				var cause *jsonx.NullError
				if !errors.As(err, &cause) {
					t.Fatal("JSON null 원인을 보존해야 합니다")
				}
			}
		})
	}
}

type bodyCustomJSON struct{ Raw string }

func (v *bodyCustomJSON) UnmarshalJSON(data []byte) error { v.Raw = string(data); return nil }

func TestRequestBodyPreservesNullableAndCustomJSON(t *testing.T) {
	body := newBodyForTest(t, t.Context(), strings.NewReader(`{"pointer":null,"map":null,"slice":null,"custom":null}`), "application/json", BodyOptions{})
	var dto struct {
		Pointer *int
		Map     map[string]int
		Slice   []int
		Custom  bodyCustomJSON
	}
	if err := body.Bind(&dto); err != nil || dto.Pointer != nil || dto.Map != nil || dto.Slice != nil || dto.Custom.Raw != "null" {
		t.Fatalf("nullable과 사용자 디코더 규칙 = %+v, 오류 = %v", dto, err)
	}
}

type bodyFormText string

func (v *bodyFormText) UnmarshalText(data []byte) error {
	*v = bodyFormText("custom:" + string(data))
	return nil
}

type BodyEmbeddedForm struct {
	Name string `form:"name"`
}

func TestRequestBodyFormUsesBodyFieldsAndKeepsPresence(t *testing.T) {
	data := "name=form&tags=a%2Cb&tags=%20c&count=&enabled=&custom=&nested=n&ignored=secret&limit=9"
	body := newBodyForTest(t, t.Context(), strings.NewReader(data), "application/x-www-form-urlencoded", BodyOptions{})
	var dto struct {
		BodyEmbeddedForm
		Tags    []string     `form:"tags"`
		Count   *int         `form:"count"`
		Missing *int         `form:"missing"`
		Enabled *bool        `form:"enabled"`
		Custom  bodyFormText `form:"custom"`
		Nested  struct {
			Value string `form:"nested"`
		}
		Ignored string `form:"-"`
		Query   int    `query:"limit"`
	}
	if err := body.Bind(&dto); err != nil {
		t.Fatal(err)
	}
	if dto.Name != "form" || !reflect.DeepEqual(dto.Tags, []string{"a,b", " c"}) || dto.Count == nil || *dto.Count != 0 || dto.Missing != nil || dto.Enabled == nil || *dto.Enabled || dto.Custom != "custom:" || dto.Nested.Value != "n" || dto.Ignored != "" || dto.Query != 0 {
		t.Fatalf("폼의 값과 누락 여부를 보존해야 합니다: %+v", dto)
	}
}

func multipartBodyForTest(t *testing.T, fileSize int) ([]byte, string) {
	t.Helper()
	var data bytes.Buffer
	writer := multipart.NewWriter(&data)
	if err := writer.WriteField("name", "upload"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "sample.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.CopyN(file, strings.NewReader(strings.Repeat("x", fileSize)), int64(fileSize)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes(), writer.FormDataContentType()
}

func assertNoBodyTempFiles(t *testing.T, folder string) {
	t.Helper()
	files, err := os.ReadDir(folder)
	if err != nil || len(files) != 0 {
		t.Fatalf("임시 파일이 남으면 안 됩니다: 파일 = %v, 오류 = %v", files, err)
	}
}

func TestRequestBodyMultipartSharesParsingAndOwnsDiskCleanup(t *testing.T) {
	folder := t.TempDir()
	t.Setenv("TMPDIR", folder)
	data, media := multipartBodyForTest(t, 128<<10)
	for _, filesFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "form_first", true: "files_first"}[filesFirst], func(t *testing.T) {
			source := &bodyProbeReader{reader: bytes.NewReader(data)}
			body := newBodyForTest(t, t.Context(), source, media, BodyOptions{MultipartMemory: 1})
			var form *multipart.Form
			if filesFirst {
				var err error
				form, err = body.MultipartForm()
				if err != nil {
					t.Fatal(err)
				}
			}
			var dto struct {
				Name string                `form:"name"`
				File *multipart.FileHeader `form:"file"`
			}
			if err := body.Bind(&dto); err != nil {
				t.Fatal(err)
			}
			reads := source.reads
			parsed, err := body.MultipartForm()
			if err != nil || parsed == nil || filesFirst && parsed != form || source.reads != reads || dto.Name != "upload" || dto.File != parsed.File["file"][0] {
				t.Fatalf("폼과 파일은 같은 파싱을 공유해야 합니다: %+v, 오류 = %v", dto, err)
			}
			file, err := dto.File.Open()
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := file.(*os.File); !ok {
				t.Fatal("실제로 디스크에 기록한 파일을 검사해야 합니다")
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := body.Cleanup(); err != nil {
				t.Fatal(err)
			}
			assertNoBodyTempFiles(t, folder)
			if _, err := dto.File.Open(); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("정리한 파일은 다시 열 수 없어야 합니다: %v", err)
			}
			if err := body.Cleanup(); err != nil || source.closes != 0 {
				t.Fatalf("반복 정리와 본문 소유권 = %v", err)
			}
		})
	}
}

func TestRequestBodyMultipartFailureIsCachedAndLeavesNoFiles(t *testing.T) {
	folder := t.TempDir()
	t.Setenv("TMPDIR", folder)
	data, media := multipartBodyForTest(t, 128<<10)
	source := &bodyProbeReader{reader: bytes.NewReader(data[:len(data)-40])}
	body := newBodyForTest(t, t.Context(), source, media, BodyOptions{MultipartMemory: 1})
	_, first := body.MultipartForm()
	assertBodyStatus(t, first, 400)
	reads := source.reads
	_, second := body.MultipartForm()
	var dto struct {
		Name string `form:"name"`
	}
	if first != second || body.Bind(&dto) != first || source.reads != reads {
		t.Fatal("실패한 multipart 파싱을 반복하면 안 됩니다")
	}
	assertNoBodyTempFiles(t, folder)
}

func TestRequestBodyMultipartEmptyAndInvalidBoundaries(t *testing.T) {
	for _, tc := range []struct{ media, data string }{
		{"multipart/form-data; boundary=probe", ""},
		{"multipart/form-data", "data"},
		{"application/json", "{}"},
	} {
		for _, formFirst := range []bool{false, true} {
			body := newBodyForTest(t, t.Context(), strings.NewReader(tc.data), tc.media, BodyOptions{})
			// 빈 폼 DTO를 먼저 읽어도 파일 조회의 잘못된 multipart를 숨기지 않습니다.
			if formFirst && tc.data == "" {
				if err := body.Bind(&struct{}{}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := body.MultipartForm()
			assertBodyStatus(t, err, 400)
		}
	}
}

func TestRequestBodyBindsMultipartFileHeaderSlices(t *testing.T) {
	data, media := multipartBodyForTest(t, 10)
	var pointerSlice struct {
		Files []*multipart.FileHeader `form:"file"`
	}
	var valueSlice struct {
		Files []multipart.FileHeader `form:"file"`
	}
	for _, target := range []any{&pointerSlice, &valueSlice} {
		body := newBodyForTest(t, t.Context(), bytes.NewReader(data), media, BodyOptions{})
		if err := body.Bind(target); err != nil {
			t.Fatal(err)
		}
	}
	if len(pointerSlice.Files) != 1 || len(valueSlice.Files) != 1 || pointerSlice.Files[0].Filename != "sample.bin" || valueSlice.Files[0].Size != 10 {
		t.Fatalf("파일 헤더 목록 바인딩: %+v %+v", pointerSlice, valueSlice)
	}
}

func TestRequestBodyLimitChecksUnknownLengthAndMultipartTrailer(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		limit      int64
		status     int
	}{
		{"exact", `{"count":2}`, 11, 0},
		{"over", `{"count":2} `, 11, 413},
		{"unlimited", `{"count":2}`, -1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newBodyForTest(t, t.Context(), &bodyProbeReader{reader: strings.NewReader(tc.data)}, "application/json", BodyOptions{MaxBytes: tc.limit})
			var dto struct{ Count int }
			err := body.Bind(&dto)
			assertBodyStatus(t, err, tc.status)
			if tc.status == 413 {
				var cause *http.MaxBytesError
				if !errors.As(err, &cause) || cause.Limit != tc.limit {
					t.Fatal("초과 원인과 한도를 보존해야 합니다")
				}
			}
		})
	}
	folder := t.TempDir()
	t.Setenv("TMPDIR", folder)
	data, media := multipartBodyForTest(t, 128<<10)
	withTrailer := append(bytes.Clone(data), []byte(strings.Repeat("x", 100))...)
	body := newBodyForTest(t, t.Context(), bytes.NewReader(withTrailer), media, BodyOptions{MaxBytes: int64(len(data) + 10), MultipartMemory: 1})
	_, err := body.MultipartForm()
	assertBodyStatus(t, err, 413)
	assertNoBodyTempFiles(t, folder)
}

type bodyFailingReader struct {
	cause error
}

func (r bodyFailingReader) Read([]byte) (int, error) { return 0, r.cause }

func TestRequestBodyKeepsIOErrorsDistinctFromBadInput(t *testing.T) {
	cause := errors.New("connection failed")
	for _, multipartInput := range []bool{false, true} {
		media := "application/json"
		if multipartInput {
			media = "multipart/form-data; boundary=probe"
		}
		body := newBodyForTest(t, t.Context(), bodyFailingReader{cause}, media, BodyOptions{})
		var err error
		if multipartInput {
			_, err = body.MultipartForm()
		} else {
			var dto struct{}
			err = body.Bind(&dto)
		}
		var input *httperr.HTTPError
		if !errors.Is(err, cause) || errors.As(err, &input) {
			t.Fatalf("I/O 원인을 입력 400으로 바꾸면 안 됩니다: %v", err)
		}
	}
}

func TestRequestBodyPreservesLateIOFailureAndCleansParsedFiles(t *testing.T) {
	folder := t.TempDir()
	t.Setenv("TMPDIR", folder)
	cause := errors.New("stream failed after a complete value")
	data, media := multipartBodyForTest(t, 128<<10)
	for _, tc := range []struct {
		media string
		data  []byte
	}{
		{"application/json", []byte(`{"count":2}`)},
		{media, data},
	} {
		body := newBodyForTest(t, t.Context(), io.MultiReader(bytes.NewReader(tc.data), bodyFailingReader{cause}), tc.media, BodyOptions{MultipartMemory: 1})
		var err error
		if tc.media == "application/json" {
			err = body.Bind(&struct{ Count int }{})
		} else {
			_, err = body.MultipartForm()
		}
		var failure *httperr.HTTPError
		if !errors.Is(err, cause) || errors.As(err, &failure) {
			t.Fatalf("완성된 값 뒤의 I/O 오류도 보존해야 합니다: %v", err)
		}
		assertNoBodyTempFiles(t, folder)
	}
}

func TestRequestBodyFormErrorsDoNotExposeInputValues(t *testing.T) {
	for _, data := range []string{"count=private-value", "count=%zz"} {
		body := newBodyForTest(t, t.Context(), strings.NewReader(data), "application/x-www-form-urlencoded", BodyOptions{})
		var dto struct {
			Count int `form:"count"`
		}
		err := body.Bind(&dto)
		assertBodyStatus(t, err, 400)
		if strings.Contains(err.Error(), "private-value") || strings.Contains(err.Error(), "%zz") {
			t.Fatal("공개 메시지에 입력값을 포함하면 안 됩니다")
		}
		if second := body.Bind(&dto); second != err {
			t.Fatal("실패한 폼 바인딩을 다시 실행하면 안 됩니다")
		}
	}
}

type bodyCancelJSON struct{ Cancel context.CancelFunc }

func (v *bodyCancelJSON) UnmarshalJSON([]byte) error { v.Cancel(); return nil }

func TestRequestBodyChecksCancellationBeforeReadingAndAfterDecoder(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	source := &bodyProbeReader{reader: strings.NewReader(`{}`)}
	body := newBodyForTest(t, ctx, source, "application/json", BodyOptions{})
	cancel()
	if err := body.Bind(&struct{}{}); !errors.Is(err, context.Canceled) || source.reads != 0 {
		t.Fatalf("취소한 요청을 읽으면 안 됩니다: %v", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	body = newBodyForTest(t, ctx, strings.NewReader(`{}`), "application/json", BodyOptions{})
	dto := bodyCancelJSON{Cancel: cancel}
	err := body.Bind(&dto)
	var failure *httperr.HTTPError
	if !errors.Is(err, context.Canceled) || errors.As(err, &failure) {
		t.Fatalf("디코더 안의 취소를 입력 오류로 바꾸면 안 됩니다: %v", err)
	}
}

func TestRequestBodyRejectsInvalidOptionsWithoutReading(t *testing.T) {
	source := &bodyProbeReader{reader: strings.NewReader(`{}`)}
	for _, args := range []struct {
		ctx     context.Context
		reader  io.Reader
		options BodyOptions
	}{
		{nil, source, BodyOptions{}},
		{t.Context(), nil, BodyOptions{}},
		{t.Context(), source, BodyOptions{MultipartMemory: -1}},
	} {
		if _, err := NewBody(args.ctx, args.reader, "application/json", args.options); err == nil {
			t.Fatal("잘못된 내부 설정을 거부해야 합니다")
		}
	}
	if source.reads != 0 {
		t.Fatal("설정 검사에서 본문을 읽으면 안 됩니다")
	}
}

func TestRequestBodiesKeepConcurrentRequestsIndependent(t *testing.T) {
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			body, err := NewBody(t.Context(), strings.NewReader(`{"count":2}`), "application/json", BodyOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			defer body.Cleanup()
			var dto struct{ Count int }
			if err := body.Bind(&dto); err != nil || dto.Count != 2 {
				t.Errorf("요청 상태를 공유하면 안 됩니다: %+v, %v", dto, err)
			}
		})
	}
	wg.Wait()
}

func TestRequestBodyOverRealHTTPKeepsQuerySeparateAndRejectsBeforeRead(t *testing.T) {
	type outcome struct{ status, reads int }
	observed := make(chan outcome, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		source := &bodyProbeReader{reader: r.Body}
		body, err := NewBody(r.Context(), source, r.Header.Get("Content-Type"), BodyOptions{})
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer body.Cleanup()
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(401)
			observed <- outcome{401, source.reads}
			return
		}
		var dto struct {
			Name string `form:"name"`
		}
		if err := body.Bind(&dto); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if err := json.NewEncoder(w).Encode(dto); err != nil {
			t.Error(err)
		}
		observed <- outcome{200, source.reads}
	}))
	defer server.Close()
	for _, authorized := range []bool{false, true} {
		// 독자적인 Reader 타입으로 Content-Length를 알 수 없는 요청을 보냅니다.
		request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/?name=from-query", &bodyProbeReader{reader: strings.NewReader("other=from-body")})
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if authorized {
			request.Header.Set("Authorization", "test")
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := <-observed
		if !authorized {
			if response.StatusCode != 401 || got.reads != 0 {
				t.Fatalf("거부한 요청의 본문을 읽으면 안 됩니다: %+v", got)
			}
		} else if response.StatusCode != 200 || got.reads == 0 || string(data) != "{\"Name\":\"\"}\n" {
			t.Fatalf("폼에 URL 쿼리를 합치면 안 됩니다: 상태 = %+v, 본문 = %s", got, data)
		}
	}
}

func TestRequestBodyOverChunkedHTTPEnforcesTheWholeLimit(t *testing.T) {
	observed := make(chan int64, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed <- r.ContentLength
		body, err := NewBody(r.Context(), r.Body, r.Header.Get("Content-Type"), BodyOptions{MaxBytes: 11})
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer body.Cleanup()
		var dto struct{ Count int }
		if err := body.Bind(&dto); err != nil {
			var failure *httperr.HTTPError
			if !errors.As(err, &failure) {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			w.WriteHeader(failure.Status)
			return
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	for _, tc := range []struct {
		data   string
		status int
	}{
		{`{"count":2}`, 204},
		{`{"count":2} `, 413},
	} {
		request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL, &bodyProbeReader{reader: strings.NewReader(tc.data)})
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if length := <-observed; length != -1 || response.StatusCode != tc.status {
			t.Fatalf("길이를 모르는 본문 한도: Content-Length = %d, 상태 = %d, 기대값 = %d", length, response.StatusCode, tc.status)
		}
	}
}
