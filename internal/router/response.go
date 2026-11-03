package router

import (
	"fmt"
	"mime"
	"reflect"
	"strings"
)

// ResponseSpec는 사용자가 명시한 응답 계약입니다. 실제 반환값을 변경하지 않습니다.
type ResponseSpec struct {
	Status      int
	Body        reflect.Type
	ContentType string
	Error       bool
}

func ValidateResponses(responses []ResponseSpec) error {
	seen := map[string]bool{}
	for _, response := range responses {
		if response.Status < 200 || response.Status > 599 || response.Error && response.Status < 400 {
			return fmt.Errorf("invalid declared response status: %d", response.Status)
		}
		media, params, err := mime.ParseMediaType(response.ContentType)
		if err != nil || len(params) != 0 || media != response.ContentType || !strings.Contains(media, "/") || response.Body == nil && !response.Error {
			return fmt.Errorf("invalid declared response content type or body: %q", response.ContentType)
		}
		key := fmt.Sprintf("%d %s", response.Status, media)
		if seen[key] {
			return fmt.Errorf("duplicate declared response: %s", key)
		}
		seen[key] = true
	}
	return nil
}
