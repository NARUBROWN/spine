package httperr

import "github.com/NARUBROWN/spine/internal/inputerr"

// InputLocation는 Spine이 입력 오류에 기록한 body, path, query 또는 header를 반환합니다.
// 입력 출처가 없으면 빈 문자열과 false를 반환합니다.
func InputLocation(err error) (string, bool) {
	return inputerr.Source(err)
}
