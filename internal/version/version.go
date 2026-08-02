// Package version holds the gotasky build version, injected at build time via -ldflags.
package version

import "runtime/debug"

// Name is the tool name shown alongside the version in -v/-h output.
const Name = "gotasky"

// Version is overwritten at build time with -ldflags "-X .../internal/version.Version=...".
// It holds the output of `git describe --tags` (see the sig9org/tasks _go.yml
// build task), or "dev" for local, unversioned builds.
var Version = "dev"

// Commit returns the VCS commit hash the running binary was built from, read
// from the build info Go's toolchain embeds automatically (since Go 1.18)
// when building from within a git checkout. It returns "unknown" if that
// information isn't available (e.g. built with -trimpath, or outside a VCS
// checkout).
func Commit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return "unknown"
}

// String returns the full version string shown by -v/-version and -h/-help:
// tool name, git-describe version, and VCS commit hash.
func String() string {
	return Name + " " + Version + " (" + Commit() + ")"
}
