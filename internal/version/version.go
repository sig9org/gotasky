// Package version holds the gotasky build version, injected at build time via -ldflags.
package version

// Name is the tool name shown alongside the version in -v/-h output.
const Name = "gotasky"

// Version is overwritten at build time with -ldflags "-X .../internal/version.Version=...".
// It holds the output of `git describe --tags --abbrev=0` (see Taskfile.yml
// build task), or "dev" for local, unversioned builds.
var Version = "dev"

// String returns the release tag shown by -v/-version and -h/-help. Commit
// IDs are intentionally omitted so the output is stable for a tagged build.
func String() string {
	return Name + " " + Version
}
