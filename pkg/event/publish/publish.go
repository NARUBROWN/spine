// Package publish defines the application-owned synchronous event publisher.
package publish

import (
	"context"
	"errors"
	"maps"
	"time"
)

// ErrUnavailable means the application has no publishing platform.
var ErrUnavailable = errors.New("event publisher is unavailable")

// Fields is the JSON object sent as an event body. Do not mutate it during Event.
type Fields map[string]any

// Publisher is injected by Spine after preparing WithEventAdapter factories.
// Event waits for every registered publishing platform's transmission result.
// A successful send is not rolled back by later handler or response errors.
// Errors and cancellation can follow broker acceptance; retries can duplicate delivery.
type Publisher interface {
	Event(context.Context, string, Fields, ...Option) error
}

// Options carries per-call metadata. Applications normally use the With functions.
// MessageID is carried in Metadata["message_id"]; no identifier is generated.
type Options struct {
	MessageID  string
	OccurredAt time.Time
	Metadata   map[string]string
}

// Option configures one immediate Event call. Later options override earlier values.
type Option func(*Options)

// WithMessageID preserves an outbox identifier across explicit retries.
func WithMessageID(id string) Option { return func(o *Options) { o.MessageID = id } }

// WithOccurredAt preserves the original event time across explicit retries.
func WithOccurredAt(at time.Time) Option { return func(o *Options) { o.OccurredAt = at } }

// WithMetadata snapshots metadata so the option can be reused by concurrent calls.
func WithMetadata(metadata map[string]string) Option {
	snapshot := maps.Clone(metadata)
	return func(o *Options) { o.Metadata = maps.Clone(snapshot) }
}
