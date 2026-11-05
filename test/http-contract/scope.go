package contractprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync/atomic"

	contract "github.com/NARUBROWN/spine/contract/http/v3"
)

type requestIDKey struct{}

type scope struct {
	ctx                    context.Context
	method, path, rawQuery string
	headers                http.Header
	body                   io.Reader
	writer                 contract.ResponseWriter
}

var _ contract.HTTPContext = (*scope)(nil)

func (s *scope) Context() context.Context        { return s.ctx }
func (s *scope) Method() string                  { return s.method }
func (s *scope) RawPath() string                 { return s.path }
func (s *scope) RawQuery() string                { return s.rawQuery }
func (s *scope) Header(name string) string       { return s.headers.Get(name) }
func (s *scope) Headers() map[string][]string    { return s.headers.Clone() }
func (s *scope) Body() io.Reader                 { return s.body }
func (s *scope) Writer() contract.ResponseWriter { return s.writer }

type hooks struct {
	native func()
	after  func()
	reads  *atomic.Int64
}

type countedReader struct {
	io.Reader
	reads *atomic.Int64
}

func (r countedReader) Read(p []byte) (int, error) {
	if r.reads != nil {
		r.reads.Add(1)
	}
	return r.Reader.Read(p)
}

var errBodyLimit = errors.New("probe body limit exceeded")

type limitedReader struct {
	source    io.Reader
	remaining int64
	err       error
}

type lengthCheckedReader struct {
	io.Reader
	remaining int64
}

func (r *lengthCheckedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.remaining -= int64(n)
	if err == io.EOF && r.remaining > 0 {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func bodyReader(source io.Reader, max, length int64, reads *atomic.Int64) io.Reader {
	if length >= 0 {
		source = &lengthCheckedReader{Reader: source, remaining: length}
	}
	reader := io.Reader(countedReader{Reader: source, reads: reads})
	if max > 0 {
		reader = &limitedReader{source: reader, remaining: max}
	}
	return reader
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.remaining < int64(len(p)) {
		p = p[:int(r.remaining)+1]
	}
	n, err := r.source.Read(p)
	if int64(n) > r.remaining {
		n = int(r.remaining)
		r.remaining = 0
		cause := errBodyLimit
		if err != nil && err != io.EOF {
			cause = errors.Join(cause, err)
		}
		r.err = &contract.RequestError{Status: 413, Cause: cause}
		return n, r.err
	}
	r.remaining -= int64(n)
	return n, err
}

type response struct {
	headers   http.Header
	committed bool
	sink      func(int, http.Header, []byte) error
}

func (w *response) SetHeader(key, value string)  { w.headers.Set(key, value) }
func (w *response) AddHeader(key, value string)  { w.headers.Add(key, value) }
func (w *response) IsCommitted() bool            { return w.committed }
func (w *response) WriteStatus(status int) error { return w.WriteBytes(status, nil) }
func (w *response) WriteString(status int, body string) error {
	return w.WriteBytes(status, []byte(body))
}
func (w *response) WriteJSON(status int, body json.RawMessage) error {
	w.SetHeader("Content-Type", "application/json")
	return w.WriteBytes(status, body)
}
func (w *response) WriteBytes(status int, body []byte) error {
	if w.committed {
		return contract.ErrInvalidState
	}
	w.committed = true
	return w.sink(status, w.headers, bytes.Clone(body))
}
