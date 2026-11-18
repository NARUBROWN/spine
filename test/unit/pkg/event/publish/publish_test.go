package publish_test

import (
	"github.com/NARUBROWN/spine/pkg/event/publish"
	"testing"
	"time"
)

func TestMetadataOptionSnapshotsInputAndEachApplication(t *testing.T) {
	input := map[string]string{"trace": "original"}
	option := publish.WithMetadata(input)
	input["trace"] = "changed"
	var first, second publish.Options
	option(&first)
	first.Metadata["trace"] = "changed again"
	option(&second)
	if second.Metadata["trace"] != "original" {
		t.Fatal("metadata was shared")
	}
	at := time.Unix(123, 0)
	publish.WithOccurredAt(at)(&second)
	publish.WithMessageID("42")(&second)
	if second.MessageID != "42" || !second.OccurredAt.Equal(at) {
		t.Fatal("outbox options changed")
	}
}
