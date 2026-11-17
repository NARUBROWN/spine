package ws

// Handshake is an immutable snapshot of the upgrade request. It holds no native
// HTTP request, connection, execution store or received message payload.
type Handshake interface {
	Path() string
	Header(string) string
	Headers() map[string][]string
	Query(string) string
	Queries() map[string][]string
	Cookie(string) (string, bool)
	Cookies() map[string]string
	RemoteAddr() string
	Host() string
	RequestURI() string
}
