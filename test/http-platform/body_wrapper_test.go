package httpplatform_test

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine"
	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/input"
)

type wrapperFormBody struct {
	Name string                `json:"name" form:"name"`
	File *multipart.FileHeader `json:"-" form:"file"`
}
type bodyWrapperController struct{}

func (*bodyWrapperController) RegisterRoutes(r *api.Registry) {
	r.POST("/body", func(body input.Body[wrapperFormBody]) (httpx.Response[string], error) {
		result := body.Name
		if body.File != nil {
			file, err := body.File.Open()
			if err != nil {
				return httpx.Response[string]{}, err
			}
			defer file.Close()
			data, err := io.ReadAll(file)
			if err != nil {
				return httpx.Response[string]{}, err
			}
			result += ":" + body.File.Filename + ":" + string(data)
		}
		return httpx.Response[string]{Body: result}, nil
	})
}
func TestBodyWrapperJSONFormAndMultipart(t *testing.T) {
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	if err := writer.WriteField("name", "Alice"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, adapter := range []string{"echo", "nethttp"} {
		t.Run(adapter, func(t *testing.T) {
			app := spine.New(spine.WithHTTPAdapter(platformFactory(adapter, nativeHooks{})))
			app.Constructor(func() *bodyWrapperController { return &bodyWrapperController{} })
			prepared := prepare(t, app, boot.HTTPOptions{})
			base, client := managedServer(t, prepared)
			for _, tc := range []struct{ media, body, want string }{
				{"application/json", `{"name":"Alice"}`, "Alice"},
				{"application/x-www-form-urlencoded", "name=Alice", "Alice"},
				{writer.FormDataContentType(), upload.String(), "Alice:hello.txt:hello"},
			} {
				t.Run(tc.media, func(t *testing.T) {
					response, err := client.Post(base+"/body?name=wrong", tc.media, strings.NewReader(tc.body))
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(response.Body)
					response.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if response.StatusCode != http.StatusOK || strings.TrimSpace(string(data)) != tc.want {
						t.Fatalf("%d %s", response.StatusCode, data)
					}
				})
			}
		})
	}
}
