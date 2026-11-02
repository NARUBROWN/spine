package bootstrap

import (
	"net"
	"sync"
)

// managedHTTPListener는 실행기와 어댑터의 종료 경로가 같은 리스너를 안전하게 정리하게 합니다.
// 특히 Serve goroutine이 시작되기 전의 초기화 취소에서도 실행기가 리스너를 닫아야 합니다.
type managedHTTPListener struct {
	net.Listener
	closeOnce sync.Once
	closeErr  error
}

func (l *managedHTTPListener) Close() error {
	l.closeOnce.Do(func() { l.closeErr = l.Listener.Close() })
	return l.closeErr
}
