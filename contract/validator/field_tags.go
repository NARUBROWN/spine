package validator

import (
	"context"
	"reflect"
	"slices"
)

type fieldTagsKey struct{}

// WithFieldTags는 이번 검증 호출의 외부 필드명에 사용할 태그 우선순위를 지정합니다.
// 규칙 태그를 바꾸지 않으며 구현체의 공유 설정을 변경하지 않습니다.
func WithFieldTags(ctx context.Context, tags ...string) context.Context {
	return context.WithValue(ctx, fieldTagsKey{}, slices.Clone(tags))
}

// FieldTags는 요청별 외부 필드명 태그를 복사해 반환합니다.
// 지정하지 않은 경우 구현체의 기본 필드명 설정을 사용합니다.
func FieldTags(ctx context.Context) ([]string, bool) {
	tags, ok := ctx.Value(fieldTagsKey{}).([]string)
	return slices.Clone(tags), ok
}

// FieldNameMapper는 원래 Go 필드와 입력 계획의 외부 이름을 연결합니다.
// 검증 구현은 매핑이 없으면 기존 태그 이름 규칙을 사용합니다.
type FieldNameMapper func(owner reflect.Type, field reflect.StructField) (string, bool)
type fieldNameMapperKey struct{}

// WithFieldNameMapper는 이번 검증 호출에서만 적용할 입력 필드 이름을 지정합니다.
// 검증기 공유 설정과 DTO 타입 및 값은 변경하지 않습니다.
func WithFieldNameMapper(ctx context.Context, mapper FieldNameMapper) context.Context {
	return context.WithValue(ctx, fieldNameMapperKey{}, mapper)
}

// MappedFieldName은 선택적 외부 이름을 조회합니다. 익명 필드의 평탄화는 구현체가 유지합니다.
func MappedFieldName(ctx context.Context, owner reflect.Type, field reflect.StructField) (string, bool) {
	mapper, _ := ctx.Value(fieldNameMapperKey{}).(FieldNameMapper)
	if mapper == nil {
		return "", false
	}
	return mapper(owner, field)
}
