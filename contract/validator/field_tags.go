package validator

import (
	"context"
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
