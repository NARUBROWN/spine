package bootstrap

import (
	"testing"

	"github.com/NARUBROWN/spine/pkg/api"
	"github.com/NARUBROWN/spine/pkg/boot"
)

func TestAbstractControllerDiagnosticsFollowCollectionCandidates(t *testing.T) {
	factory := func() api.Controller { panic("validation must not execute a factory") }
	for _, tc := range []struct {
		name       string
		candidates []any
		wantPath   string
	}{
		{"legacy", nil, "Constructors[0]"},
		{"excluded custom", []any{}, ""},
		{"explicit candidate", []any{factory}, "ControllerConstructors[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := controllerProviderIssues(Config{HTTP: &boot.HTTPOptions{}, Constructors: []any{factory}, ControllerConstructors: tc.candidates})
			if tc.wantPath == "" {
				if len(issues) != 0 {
					t.Fatalf("excluded provider diagnostics: %+v", issues)
				}
			} else if len(issues) != 1 || issues[0].Code != "HTTP_CONTROLLER_PROVIDER_ABSTRACT" || issues[0].Path != tc.wantPath {
				t.Fatalf("issues=%+v wantPath=%s", issues, tc.wantPath)
			}
		})
	}
}
