package core

/*
Interceptor는 실행 흐름의 횡단 관심사를 처리하는 계약이다.
Controller/Invoker/Resolver는 Interceptor를 몰라야 한다.
*/
type Interceptor interface {
	/*
		PreHandle은 Controller 호출 전에 실행된다.
		여기서 에러를 반환하면 실행을 중단한다.
	*/
	PreHandle(ctx ExecutionContext, meta HandlerMeta) error

	/*
		PostHandle은 ReturnValueHandler 처리 후 실행된다.
		실패해도 전체 파이프라인 실패로 만들지 않는다.
	*/
	PostHandle(ctx ExecutionContext, meta HandlerMeta)

	/*
		AfterCompletion은 성공/실패와 관계없이 마지막에 호출된다.
		err는 Pipeline 실행 중 발생한 최종 에러
	*/
	AfterCompletion(ctx ExecutionContext, meta HandlerMeta, err error)
}
