// Package constructor는 하나의 Constructor 호출에서 객체의 역할을 선언합니다.
package constructor

// Registration은 역할과 생성 규칙을 전달하는 표식입니다.
// 생성자를 실행하거나 별도의 DI 컨테이너를 만들지 않습니다.
type Registration struct {
	role  string
	value any
}

// Role은 등록한 객체의 역할을 반환합니다.
func (r Registration) Role() string { return r.role }

// Value는 객체의 타입 표식 또는 기존 생성자 함수를 반환합니다.
func (r Registration) Value() any { return r.value }

// Custom은 공통 생성 계약을 따르는 객체 또는 기존 생성자 함수를 등록합니다.
func Custom(value any) Registration { return Registration{role: "Custom", value: value} }

// Repository는 공통 생성 계약을 따르는 저장소를 등록합니다.
func Repository(prototype any) Registration {
	return Registration{role: "Repository", value: prototype}
}

// Service는 공통 생성 계약을 따르는 서비스를 등록합니다.
func Service(prototype any) Registration { return Registration{role: "Service", value: prototype} }

// Controller는 공통 생성 계약과 api.Controller를 따르는 컨트롤러를 등록합니다.
func Controller(prototype any) Registration {
	return Registration{role: "Controller", value: prototype}
}

// Interceptor는 공통 생성 계약과 core.Interceptor를 따르는 인터셉터를 등록합니다.
// 실행 범위와 순서는 InterceptorFor 또는 경로 옵션으로 지정합니다.
func Interceptor(prototype any) Registration {
	return Registration{role: "Interceptor", value: prototype}
}
