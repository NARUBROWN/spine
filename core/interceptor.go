package core

import "github.com/NARUBROWN/spine/pkg/spine"

/*
Interceptor는 실행 흐름의 횡단 관심사를 처리하는 계약이다.
컨트롤러, Invoker, Resolver는 Interceptor를 몰라야 한다.
*/
type Interceptor interface {
	/*
		PreHandle은 컨트롤러 호출 전에 실행된다.
		여기서 오류를 반환하면 실행을 중단한다.
	*/
	PreHandle(ctx spine.Ctx, meta HandlerMeta) error

	/*
		PostHandle은 ReturnValueHandler 처리 후 실행된다.
		실패해도 전체 파이프라인 실패로 만들지 않는다.
	*/
	PostHandle(ctx spine.Ctx, meta HandlerMeta)

	/*
		BeforeResponse는 핸들러 실행과 후속 훅이 끝난 뒤, 응답을 쓰기 전에 실행된다.
		PreHandle을 성공한 Interceptor만 역순으로 호출된다.
		executionErr는 이 시점까지 발생한 최종 오류이며, 구현체가 반환한 오류는
		최종 실행 오류에 포함된다.
	*/
	BeforeResponse(ctx spine.Ctx, meta HandlerMeta, executionErr error) error

	/*
		AfterCompletion은 성공/실패와 관계없이 마지막에 호출된다.
		err는 파이프라인 실행 중 발생한 최종 오류
	*/
	AfterCompletion(ctx spine.Ctx, meta HandlerMeta, err error)
}

/*
WebSocketHandshakeInterceptor는 선택적 WebSocket handshake 사전 처리 계약입니다.
구현한 인터셉터만 연결 슬롯 예약과 HTTP upgrade 전에 호출됩니다. 기존 Interceptor의
PreHandle은 메시지마다 기존 시점에 계속 호출되므로 호출 횟수와 트랜잭션 의미가 바뀌지 않습니다.
*/
type WebSocketHandshakeInterceptor interface {
	PreHandshake(ctx spine.Ctx, meta HandlerMeta) error
}
