// Package constructor는 하나의 Constructor 호출에서 객체의 역할을 선언합니다.
package constructor

import "github.com/NARUBROWN/spine/internal/componentregistration"

// Registration은 생성자 등록에 전달하는 역할 표식입니다.
type Registration = componentregistration.Registration

// Custom은 공통 생성 계약을 따르는 객체 또는 기존 생성자 함수를 등록합니다.
func Custom(value any) Registration { return componentregistration.New("Custom", value) }

// Repository는 공통 생성 계약을 따르는 저장소를 등록합니다.
func Repository(prototype any) Registration {
	return componentregistration.New("Repository", prototype)
}

// Service는 공통 생성 계약을 따르는 서비스를 등록합니다.
func Service(prototype any) Registration { return componentregistration.New("Service", prototype) }

// Controller는 공통 생성 계약과 api.Controller를 따르는 컨트롤러를 등록합니다.
func Controller(prototype any) Registration {
	return componentregistration.New("Controller", prototype)
}

// Interceptor는 공통 생성 계약과 core.Interceptor를 따르는 인터셉터를 등록합니다.
// 실행 범위와 순서는 InterceptorFor 또는 경로 옵션으로 지정합니다.
func Interceptor(prototype any) Registration {
	return componentregistration.New("Interceptor", prototype)
}
