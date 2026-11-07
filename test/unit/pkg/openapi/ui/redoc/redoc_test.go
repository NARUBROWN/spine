package redoc

import (
	"strings"
	"testing"
)

func TestDownloadUsesOriginalOpenAPI(t *testing.T) {
	resources, err := New(Options{})("/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range resources {
		if strings.HasSuffix(resource.Path, "/init.js") {
			if !strings.Contains(resource.Body, "Redoc.init(presentation,{downloadDefinitionUrl:documentURL.href,") {
				t.Fatal("ReDoc must download the original OpenAPI document")
			}
			return
		}
	}
	t.Fatal("missing ReDoc initializer")
}
