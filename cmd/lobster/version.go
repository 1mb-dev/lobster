package main

import (
	"runtime/debug"
	"strings"
)

// version is set at build time via ldflags: -X main.version=<git describe>.
var version = "dev"

// resolveVersion returns the version without a leading "v": the ldflags value
// when one was set, else the module version `go install` records, else "dev".
func resolveVersion(ldflags string, info *debug.BuildInfo, ok bool) string {
	if ldflags != "" && ldflags != "dev" {
		return strings.TrimPrefix(ldflags, "v")
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// currentVersion resolves this binary's version.
func currentVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

// displayVersion prefixes a release version with exactly one "v".
func displayVersion(v string) string {
	if v == "dev" {
		return v
	}
	return "v" + v
}

// userAgent is the default User-Agent header.
func userAgent(v string) string {
	return "lobster/" + v
}
