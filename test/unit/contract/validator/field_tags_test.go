package validator

import (
	"context"
	"reflect"
	"testing"
)

func TestFieldTagsAreScopedAndCopied(t *testing.T) {
	parent := context.Background()
	tags := []string{"header"}
	ctx := WithFieldTags(parent, tags...)
	tags[0] = "json"
	first, ok := FieldTags(ctx)
	if !ok || !reflect.DeepEqual(first, []string{"header"}) {
		t.Fatal("입력 태그를 복사해야 합니다")
	}
	first[0] = "form"
	second, _ := FieldTags(ctx)
	if second[0] != "header" {
		t.Fatal("반환한 태그를 변경해도 요청 설정을 보존해야 합니다")
	}
	if _, ok := FieldTags(parent); ok {
		t.Fatal("부모 컨텍스트의 이름 정책을 변경하면 안 됩니다")
	}
}
