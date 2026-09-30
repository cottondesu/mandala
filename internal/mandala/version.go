package mandala

import "runtime/debug"

func currentModuleVersion() string {
	return moduleVersionFromBuildInfo(debug.ReadBuildInfo())
}

func moduleVersionFromBuildInfo(info *debug.BuildInfo, ok bool) string {
	if !ok {
		return "(devel)"
	}
	return normalizeModuleVersion(info.Main.Version)
}

func normalizeModuleVersion(version string) string {
	if version == "" || version == "(devel)" {
		return "(devel)"
	}
	return version
}

func renderVersion(version string) string {
	return "mandala " + normalizeModuleVersion(version) + "\n"
}
