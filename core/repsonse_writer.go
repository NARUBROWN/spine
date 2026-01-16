package core

type ResponseWriter interface {
	WriteJSON(status int, value any) error
	WriteString(status int, value string) error
}
