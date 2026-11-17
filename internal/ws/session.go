package ws

import (
	"context"
	"net/http"
	"net/url"
)

// session is owned by the connection runtime, never by execution.State.
type session struct {
	ctx     context.Context
	id      string
	request *webSocketRequestSnapshot
}

type webSocketRequestSnapshot struct {
	path       string
	headers    http.Header
	queries    url.Values
	cookies    map[string]string
	remoteAddr string
	host       string
	requestURI string
}

func newWebSocketRequestSnapshot(req *http.Request) *webSocketRequestSnapshot {
	if req == nil {
		return &webSocketRequestSnapshot{
			headers: make(http.Header),
			queries: make(url.Values),
			cookies: make(map[string]string),
		}
	}
	queries := make(url.Values, len(req.URL.Query()))
	for key, values := range req.URL.Query() {
		queries[key] = append([]string(nil), values...)
	}
	cookies := make(map[string]string)
	for _, cookie := range req.Cookies() {
		if _, exists := cookies[cookie.Name]; !exists {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return &webSocketRequestSnapshot{
		path:       req.URL.Path,
		headers:    req.Header.Clone(),
		queries:    queries,
		cookies:    cookies,
		remoteAddr: req.RemoteAddr,
		host:       req.Host,
		requestURI: req.RequestURI,
	}
}

func (s *webSocketRequestSnapshot) Path() string              { return s.path }
func (s *webSocketRequestSnapshot) Header(name string) string { return s.headers.Get(name) }
func (s *webSocketRequestSnapshot) Headers() map[string][]string {
	return s.headers.Clone()
}
func (s *webSocketRequestSnapshot) Query(name string) string { return s.queries.Get(name) }
func (s *webSocketRequestSnapshot) Queries() map[string][]string {
	copyValues := make(map[string][]string, len(s.queries))
	for key, values := range s.queries {
		copyValues[key] = append([]string(nil), values...)
	}
	return copyValues
}
func (s *webSocketRequestSnapshot) Cookie(name string) (string, bool) {
	value, ok := s.cookies[name]
	return value, ok
}
func (s *webSocketRequestSnapshot) Cookies() map[string]string {
	copyCookies := make(map[string]string, len(s.cookies))
	for name, value := range s.cookies {
		copyCookies[name] = value
	}
	return copyCookies
}
func (s *webSocketRequestSnapshot) RemoteAddr() string { return s.remoteAddr }
func (s *webSocketRequestSnapshot) Host() string       { return s.host }
func (s *webSocketRequestSnapshot) RequestURI() string { return s.requestURI }
