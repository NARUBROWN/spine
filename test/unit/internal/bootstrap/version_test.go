package bootstrap

import (
	"runtime/debug"
	"testing"
)

func TestRuntimeVersionUsesPublishedModuleMetadata(t *testing.T) {
	for _, version := range []string{"v0.6.6-preview.1", "v0.6.6-rc.2", "v0.6.6"} {
		for _, info := range []*debug.BuildInfo{
			{Main: debug.Module{Path: spineModulePath, Version: version}},
			{Main: debug.Module{Path: "example.com/app"}, Deps: []*debug.Module{{Path: spineModulePath, Version: version}}},
		} {
			if got := spineVersionFromBuild(info); got != version {
				t.Errorf("설치한 모듈 버전 = %q, 표시 = %q", version, got)
			}
		}
	}
}

func TestRuntimeVersionDoesNotAdvertiseLocalSourceAsStable(t *testing.T) {
	for _, info := range []*debug.BuildInfo{
		{},
		{Main: debug.Module{Path: spineModulePath, Version: "(devel)"}},
		{Deps: []*debug.Module{{Path: spineModulePath, Version: "v0.6.5", Replace: &debug.Module{Path: "../spine"}}}},
	} {
		if got := spineVersionFromBuild(info); got != spineVersion+"-dev" {
			t.Errorf("개발 소스를 정식 버전으로 표시했습니다: %q", got)
		}
	}
}
