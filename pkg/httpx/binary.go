package httpx

type Binary struct {
	ContentType string
	Data        []byte
	Options     ResponseOptions
}
