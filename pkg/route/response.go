package route

import (
	"reflect"

	"github.com/NARUBROWN/spine/internal/router"
)

// Response는 동적 응답의 상태와 본문 타입을 명세에 선언합니다. 실제 응답은 변경하지 않습니다.
// 문자열은 text/plain, 나머지는 application/json을 기본으로 기술합니다.
func Response[T any](status int) router.RouteOption {
	media := "application/json"
	if reflect.TypeFor[T]().Kind() == reflect.String {
		media = "text/plain"
	}
	return ResponseAs[T](status, media)
}

// ResponseAs는 응답의 Content-Type도 명시합니다. 바이너리는 []byte와 실제 미디어 타입을 지정합니다.
func ResponseAs[T any](status int, contentType string) router.RouteOption {
	return func(spec *router.RouteSpec) {
		spec.Responses = append(spec.Responses, router.ResponseSpec{Status: status, Body: reflect.TypeFor[T](), ContentType: contentType})
	}
}

// ErrorResponse는 프레임워크의 JSON 오류 본문을 상태 코드에 연결합니다.
func ErrorResponse(status int) router.RouteOption {
	return func(spec *router.RouteSpec) {
		spec.Responses = append(spec.Responses, router.ResponseSpec{Status: status, ContentType: "application/json", Error: true})
	}
}
