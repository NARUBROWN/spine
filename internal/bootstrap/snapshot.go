package bootstrap

import "slices"

// snapshotConfig는 등록 목록과 옵션 값을 복사합니다.
// 생성자, 인터셉터 인스턴스와 사용자 제공 transport/factory는 복제하지 않습니다.
func snapshotConfig(config Config) Config {
	config.Constructors = slices.Clone(config.Constructors)
	config.Routes = slices.Clone(config.Routes)
	for i := range config.Routes {
		config.Routes[i].Interceptors = slices.Clone(config.Routes[i].Interceptors)
	}
	config.Interceptors = slices.Clone(config.Interceptors)
	config.ScopedInterceptors = slices.Clone(config.ScopedInterceptors)
	config.CustomTransports = slices.Clone(config.CustomTransports)
	if config.ConsumerRegistry != nil {
		config.ConsumerRegistry = config.ConsumerRegistry.Snapshot()
	}
	if config.WebSocketRegistry != nil {
		config.WebSocketRegistry = config.WebSocketRegistry.Snapshot()
	}
	config.HTTP = snapshotValue(config.HTTP)
	if config.HTTP != nil {
		config.HTTP.OpenAPI.UI = slices.Clone(config.HTTP.OpenAPI.UI)
		config.HTTP.WebSocket.AllowedOrigins = slices.Clone(config.HTTP.WebSocket.AllowedOrigins)
		config.HTTP.WebSocket.TrustedProxyCIDRs = slices.Clone(config.HTTP.WebSocket.TrustedProxyCIDRs)
	}
	config.EventAdapters = slices.Clone(config.EventAdapters)
	return config
}

func snapshotValue[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
