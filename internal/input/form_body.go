package input

import (
	"context"
	"encoding"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"reflect"
)

// FormFileType은 폼 바인더가 파일 파트로 받는 타입을 판별합니다.
func FormFileType(t reflect.Type) bool {
	return t == reflect.TypeFor[*multipart.FileHeader]() ||
		t == reflect.TypeFor[[]*multipart.FileHeader]() ||
		t == reflect.TypeFor[[]multipart.FileHeader]()
}

// CloneFileHeader는 메타데이터를 복사하고 Open에 필요한 업로드 참조를 유지합니다.
func CloneFileHeader(file *multipart.FileHeader) *multipart.FileHeader {
	if file == nil {
		return nil
	}
	header := *file
	header.Header = textproto.MIMEHeader(http.Header(file.Header).Clone())
	return &header
}

// bindBodyForm은 사전 검사와 명세가 사용하는 폼 필드 목록으로 본문 값만 매핑합니다.
// 쿼리의 쉼표 분리 규칙을 폼 값에 적용하지 않습니다.
func bindBodyForm(ctx context.Context, target any, values map[string][]string, files map[string][]*multipart.FileHeader) error {
	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("form target must be a non-nil pointer to a struct")
	}
	value = value.Elem()
	fields, conflicts := BodyFields(value.Type(), true)
	if len(conflicts) > 0 {
		return fmt.Errorf("form repeats input key %q", conflicts[0])
	}
	for _, field := range fields {
		if err := ctx.Err(); err != nil {
			return err
		}
		fieldValue := value.FieldByIndex(field.Index)
		if fieldValue.Type() == reflect.TypeFor[multipart.FileHeader]() {
			return fmt.Errorf("form file field %s must use a pointer or slice", field.Name)
		}
		if headers := files[field.Name]; len(headers) > 0 {
			switch fieldValue.Type() {
			case reflect.TypeFor[*multipart.FileHeader]():
				fieldValue.Set(reflect.ValueOf(CloneFileHeader(headers[0])))
				continue
			case reflect.TypeFor[[]*multipart.FileHeader]():
				cloned := make([]*multipart.FileHeader, len(headers))
				for i, header := range headers {
					cloned[i] = CloneFileHeader(header)
				}
				fieldValue.Set(reflect.ValueOf(cloned))
				continue
			case reflect.TypeFor[[]multipart.FileHeader]():
				copy := make([]multipart.FileHeader, len(headers))
				for i, header := range headers {
					if header != nil {
						copy[i] = *CloneFileHeader(header)
					}
				}
				fieldValue.Set(reflect.ValueOf(copy))
				continue
			}
		}
		raw, present := values[field.Name]
		if !present || len(raw) == 0 {
			continue
		}
		if !supported(field.Type) {
			return fmt.Errorf("unsupported form field %s: %s", field.Name, field.Type)
		}
		if len(raw) > 1 && !queryList(field.Type) {
			return fmt.Errorf("multiple values for form field %s", field.Name)
		}
		if err := setFormValue(fieldValue, raw); err != nil {
			return fmt.Errorf("invalid form field %s: %w", field.Name, err)
		}
	}
	return ctx.Err()
}

func setFormValue(value reflect.Value, raw []string) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return setFormValue(value.Elem(), raw)
	}
	if decoder, ok := value.Addr().Interface().(encoding.TextUnmarshaler); ok {
		return decoder.UnmarshalText([]byte(raw[0]))
	}
	if value.Kind() == reflect.Slice {
		value.Set(reflect.MakeSlice(value.Type(), len(raw), len(raw)))
		for i, item := range raw {
			if err := setFormValue(value.Index(i), []string{item}); err != nil {
				return err
			}
		}
		return nil
	}
	// 기존 폼 바인딩처럼 명시한 빈 숫자와 bool은 0과 false로 읽습니다.
	// 누락된 키는 이 함수에 전달하지 않으므로 포인터의 nil과 구분됩니다.
	if raw[0] == "" && value.Kind() != reflect.String {
		switch value.Kind() {
		case reflect.Bool:
			raw = []string{"false"}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
			reflect.Float32, reflect.Float64:
			raw = []string{"0"}
		}
	}
	return set(value, raw)
}
