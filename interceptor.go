package spine

import "github.com/NARUBROWN/spine/core"

/*
Interceptor는 실행 흐름을 감사는 횡단 관심사 처리 계약

Interceptor는 Handler 호출 전후에 개입할 수 있으며,
반드시 next를 호출해야 다음 단계로 실행이 전달됨

실행 순서, 중단 여부, 전후 처리의 책임은
Pipeline이 아닌 Interceptor 구현체에 있다.
*/
type Interceptor interface {
	/*
		Around는 실행을 감싸는 유일한 진입점

		next를 호출하면 다음 Interceptor 또는 최종 Handler 실행으로 제어가 전달된다.
		next를 호출하지 않으면 실행은 중단된다.
	*/
	Around(ctx core.ExecutionContext, next Next) error
}

// Next는 다음 실행 단계로 제어를 전달하는 함수입니다.
type Next func() error
