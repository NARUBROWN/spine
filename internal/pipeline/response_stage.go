package pipeline

import (
	"encoding/json"
	"fmt"

	"github.com/NARUBROWN/spine/core"
	"github.com/NARUBROWN/spine/internal/handler"
)

type stagedBodyKind uint8

const (
	stagedBodyNone stagedBodyKind = iota
	stagedBodyStatus
	stagedBodyJSON
	stagedBodyString
	stagedBodyBytes
)

type stagedHeaderOperation struct {
	add   bool
	key   string
	value string
}

type stagedResponseSnapshot struct {
	headerOperations int
	bodyKind         stagedBodyKind
	status           int
	jsonBody         json.RawMessage
	stringBody       string
	bytesBody        []byte
	prepared         bool
}

// stagedResponseWriter prepares a complete response without exposing it to the
// transport. This lets the pipeline finish every framework-controlled failure
// point before BeforeResponse commits request-scoped transactions.
type stagedResponseWriter struct {
	target           core.ResponseWriter
	headerOperations []stagedHeaderOperation
	bodyKind         stagedBodyKind
	status           int
	jsonBody         json.RawMessage
	stringBody       string
	bytesBody        []byte
	prepared         bool
	checkpoint       *stagedResponseSnapshot
}

func newStagedResponseWriter(target core.ResponseWriter) *stagedResponseWriter {
	return &stagedResponseWriter{target: target}
}

func (w *stagedResponseWriter) SetHeader(key, value string) {
	w.headerOperations = append(w.headerOperations, stagedHeaderOperation{key: key, value: value})
}

func (w *stagedResponseWriter) AddHeader(key, value string) {
	w.headerOperations = append(w.headerOperations, stagedHeaderOperation{add: true, key: key, value: value})
}

func (w *stagedResponseWriter) IsCommitted() bool {
	return w.prepared || w.target.IsCommitted()
}

func (w *stagedResponseWriter) WriteStatus(status int) error {
	if err := handler.ValidateStatus(status); err != nil {
		return err
	}
	w.setPreparedResponse(stagedBodyStatus, status)
	return nil
}

func (w *stagedResponseWriter) WriteJSON(status int, value json.RawMessage) error {
	if err := handler.ValidateStatus(status); err != nil {
		return err
	}
	if value == nil {
		value = json.RawMessage("null")
	}
	if !json.Valid(value) {
		return fmt.Errorf("serialize JSON response: invalid JSON payload")
	}
	w.setPreparedResponse(stagedBodyJSON, status)
	w.jsonBody = append(w.jsonBody[:0], value...)
	return nil
}

func (w *stagedResponseWriter) WriteString(status int, value string) error {
	if err := handler.ValidateStatus(status); err != nil {
		return err
	}
	w.setPreparedResponse(stagedBodyString, status)
	w.stringBody = value
	return nil
}

func (w *stagedResponseWriter) WriteBytes(status int, value []byte) error {
	if err := handler.ValidateStatus(status); err != nil {
		return err
	}
	w.setPreparedResponse(stagedBodyBytes, status)
	w.bytesBody = append(w.bytesBody[:0], value...)
	return nil
}

func (w *stagedResponseWriter) setPreparedResponse(kind stagedBodyKind, status int) {
	w.bodyKind = kind
	w.status = status
	w.jsonBody = nil
	w.stringBody = ""
	w.bytesBody = nil
	w.prepared = true
}

func (w *stagedResponseWriter) checkpointSuccessResponse() {
	w.checkpoint = &stagedResponseSnapshot{
		headerOperations: len(w.headerOperations),
		bodyKind:         w.bodyKind,
		status:           w.status,
		jsonBody:         append(json.RawMessage(nil), w.jsonBody...),
		stringBody:       w.stringBody,
		bytesBody:        append([]byte(nil), w.bytesBody...),
		prepared:         w.prepared,
	}
}

func (w *stagedResponseWriter) rollbackSuccessResponse() {
	if w.checkpoint == nil {
		return
	}
	snapshot := w.checkpoint
	w.headerOperations = w.headerOperations[:snapshot.headerOperations]
	w.bodyKind = snapshot.bodyKind
	w.status = snapshot.status
	w.jsonBody = append(w.jsonBody[:0], snapshot.jsonBody...)
	w.stringBody = snapshot.stringBody
	w.bytesBody = append(w.bytesBody[:0], snapshot.bytesBody...)
	w.prepared = snapshot.prepared
	w.checkpoint = nil
}

func (w *stagedResponseWriter) flush() error {
	for _, operation := range w.headerOperations {
		if operation.add {
			w.target.AddHeader(operation.key, operation.value)
		} else {
			w.target.SetHeader(operation.key, operation.value)
		}
	}

	if !w.prepared {
		return nil
	}

	switch w.bodyKind {
	case stagedBodyStatus:
		return w.target.WriteStatus(w.status)
	case stagedBodyJSON:
		// json.RawMessage is already serialized, so the transport cannot hit a
		// second application-controlled JSON serialization failure after commit.
		return w.target.WriteJSON(w.status, json.RawMessage(w.jsonBody))
	case stagedBodyString:
		return w.target.WriteString(w.status, w.stringBody)
	case stagedBodyBytes:
		return w.target.WriteBytes(w.status, w.bytesBody)
	default:
		return nil
	}
}
