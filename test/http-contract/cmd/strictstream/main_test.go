package main

import (
	"reflect"
	"testing"
)

func TestSummarize(t *testing.T) {
	output := []byte(`non-JSON diagnostic
{"Action":"fail","Test":"TestParent"}
{"Action":"fail","Test":"TestParent/child"}
{"Action":"fail","Test":"TestParent/child"}
{"Action":"pass","Test":"TestParent/other"}
{"Action":"skip","Test":"TestSkipped"}
{"Action":"pass","Package":"example.com/fixture"}
`)
	got, err := summarize(output)
	want := stats{LeafPass: 1, LeafFail: 2, Skip: 1, FailedCases: []string{"TestParent/child"}, PackagePass: 1}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("summarize = %+v, %v; want %+v", got, err, want)
	}
	if _, err := summarize([]byte("{invalid")); err == nil {
		t.Fatal("invalid test event was accepted")
	}
}
