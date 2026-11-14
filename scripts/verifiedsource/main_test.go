package main

import (
	"bytes"
	"testing"
)

func TestVerify(t *testing.T) {
	for _, test := range []struct {
		results string
		success bool
	}{
		{`{"unit":{"result":"success"},"race":{"result":"success"}}`, true},
		{`{"unit":{"result":"success"},"race":{"result":"skipped"}}`, false},
		{`{"unit":{"result":"failure"}}`, false},
		{`{"unit":{"result":"cancelled"}}`, false},
		{`{"unit":{}}`, false},
		{`broken`, false},
	} {
		var output bytes.Buffer
		err := verify(test.results, "abcdef", &output)
		if test.success {
			if err != nil || output.String() != "sha=abcdef\n" {
				t.Fatalf("%s: %v, %q", test.results, err, output.String())
			}
		} else if err == nil || output.Len() != 0 {
			t.Fatalf("%s: %v, %q", test.results, err, output.String())
		}
	}
}
