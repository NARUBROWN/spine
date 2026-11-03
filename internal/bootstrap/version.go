package bootstrap

import "runtime/debug"

const spineModulePath = "github.com/NARUBROWN/spine"

// runtimeSpineVersion은 설치한 모듈의 Preview·RC·정식 버전을 그대로 표시합니다.
// 소스 checkout이나 로컬 replace는 아직 발행한 정식 버전으로 표시하지 않습니다.
func runtimeSpineVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return spineVersion + "-dev"
	}
	return spineVersionFromBuild(info)
}

func spineVersionFromBuild(info *debug.BuildInfo) string {
	modules := append([]*debug.Module{&info.Main}, info.Deps...)
	for _, module := range modules {
		if module.Path != spineModulePath {
			continue
		}
		if module.Replace != nil {
			module = module.Replace
		}
		if module.Path == spineModulePath && module.Version != "" && module.Version != "(devel)" {
			return module.Version
		}
	}
	return spineVersion + "-dev"
}
