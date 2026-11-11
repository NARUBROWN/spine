package input

import (
	"bytes"
	"encoding/xml"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

type bodyMediaDecoderProbe struct{ calls int }

func (p *bodyMediaDecoderProbe) UnmarshalJSON([]byte) error {
	p.calls++
	return nil
}

func (p *bodyMediaDecoderProbe) UnmarshalXML(decoder *xml.Decoder, _ xml.StartElement) error {
	p.calls++
	return decoder.Skip()
}

func TestRequestBodyMediaRestrictionsKeepEmptyBodiesOptional(t *testing.T) {
	for _, media := range []string{"", "text/plain", "application/json; =bad"} {
		for _, allowed := range []struct {
			name  string
			media []string
		}{
			{"json_only", []string{"application/json"}},
			{"no_formats", []string{}},
		} {
			t.Run(media+"/"+allowed.name, func(t *testing.T) {
				body := newBodyForTest(t, t.Context(), strings.NewReader(""), media, BodyOptions{})
				var target bodyMediaDecoderProbe
				if err := body.Bind(&target, allowed.media); err != nil {
					t.Fatalf("empty optional body must ignore Content-Type and media restrictions: %v", err)
				}
				if target.calls != 0 {
					t.Fatalf("empty body invoked the decoder %d times", target.calls)
				}
			})
		}
	}
}

func TestRequestBodyMediaRestrictionsRunBeforeDecoders(t *testing.T) {
	for _, tc := range []struct {
		name, media, data string
		allowed           []string
	}{
		{"json", "application/json", `{"value":"blocked"}`, []string{"application/xml"}},
		{"xml", "application/xml", `<value>blocked</value>`, []string{"application/json"}},
		{"text_xml", "text/xml", `<value>blocked</value>`, []string{"application/json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newBodyForTest(t, t.Context(), strings.NewReader(tc.data), tc.media, BodyOptions{})
			var target bodyMediaDecoderProbe
			assertBodyStatus(t, body.Bind(&target, tc.allowed), http.StatusUnsupportedMediaType)
			if target.calls != 0 {
				t.Fatalf("disallowed media invoked the decoder %d times", target.calls)
			}
		})
	}

	t.Run("allowed_json_with_parameters", func(t *testing.T) {
		body := newBodyForTest(t, t.Context(), strings.NewReader(`{}`), "Application/JSON; charset=utf-8", BodyOptions{})
		var target bodyMediaDecoderProbe
		if err := body.Bind(&target, []string{"application/json"}); err != nil || target.calls != 1 {
			t.Fatalf("allowed media must reach the decoder once: calls=%d, error=%v", target.calls, err)
		}
	})
}

func TestRequestBodyExplicitEmptyMediaRestrictionRejectsNonemptyBodies(t *testing.T) {
	multipartData, multipartMedia := multipartBodyForTest(t, 1)
	for _, tc := range []struct{ name, media, data string }{
		{"json", "application/json", `{}`},
		{"xml", "application/xml", `<value/>`},
		{"text_xml", "text/xml", `<value/>`},
		{"form", "application/x-www-form-urlencoded", "name=value"},
		{"multipart", multipartMedia, string(multipartData)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newBodyForTest(t, t.Context(), strings.NewReader(tc.data), tc.media, BodyOptions{})
			var target bodyMediaDecoderProbe
			assertBodyStatus(t, body.Bind(&target, []string{}), http.StatusUnsupportedMediaType)
			if target.calls != 0 {
				t.Fatalf("an empty allowlist invoked the decoder %d times", target.calls)
			}
		})
	}
}

func TestRequestBodyMediaRestrictionsApplyAfterMultipartLookup(t *testing.T) {
	data, media := multipartBodyForTest(t, 1)
	for _, tc := range []struct {
		name    string
		allowed []string
		status  int
	}{
		{"json_only", []string{"application/json"}, http.StatusUnsupportedMediaType},
		{"no_formats", []string{}, http.StatusUnsupportedMediaType},
		{"multipart_allowed", []string{"multipart/form-data"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &bodyProbeReader{reader: bytes.NewReader(data)}
			body := newBodyForTest(t, t.Context(), source, media, BodyOptions{})
			form, err := body.MultipartForm()
			if err != nil || form == nil || len(form.File["file"]) != 1 {
				t.Fatalf("file lookup must parse the multipart body first: form=%+v, error=%v", form, err)
			}
			reads := source.reads
			target := struct {
				Name string                `form:"name"`
				File *multipart.FileHeader `form:"file"`
			}{Name: "untouched"}
			assertBodyStatus(t, body.Bind(&target, tc.allowed), tc.status)
			if source.reads != reads {
				t.Fatal("binding after file lookup reread the consumed stream")
			}
			if tc.status == 0 {
				if target.Name != "upload" || target.File == nil {
					t.Fatalf("allowed multipart must bind cached values despite stream EOF: %+v", target)
				}
			} else if target.Name != "untouched" || target.File != nil {
				t.Fatalf("disallowed multipart mutated the target: %+v", target)
			}
		})
	}
}
