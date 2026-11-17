package core

import (
	"context"

	"github.com/NARUBROWN/spine/pkg/ws"
)

// WebSocketInterceptor는 하나의 수신 프레임에 적용되는 훅입니다.
type WebSocketInterceptor interface {
	// PreMessage는 메시지를 변환하고 검증하기 전에 호출됩니다.
	PreMessage(ctx ws.MessageContext, meta HandlerMeta) error
	// PostMessage는 핸들러가 성공한 뒤 역순으로 호출됩니다.
	PostMessage(ctx ws.MessageContext, meta HandlerMeta)
	// BeforeMessageCompletion은 사전 훅이 성공한 인터셉터를 역순으로 실행합니다.
	BeforeMessageCompletion(ctx ws.MessageContext, meta HandlerMeta, err error) error
	// AfterMessageCompletion은 프레임이 끝날 때 최종 오류를 받습니다.
	AfterMessageCompletion(ctx ws.MessageContext, meta HandlerMeta, err error)
}

// WebSocketUpgradeInterceptor는 연결 시도마다 HTTP 업그레이드 전에 실행됩니다.
// BeforeUpgrade가 오류를 반환하면 연결을 거부합니다. 훅 컨텍스트는
// 업그레이드 검사 단계가 끝날 때 취소됩니다.
type WebSocketUpgradeInterceptor interface {
	BeforeUpgrade(ctx context.Context, request ws.Handshake, meta HandlerMeta) error
}
