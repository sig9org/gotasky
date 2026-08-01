// Package version holds the gotasky build version, injected at build time via -ldflags.
package version

// Version is overwritten at build time with -ldflags "-X .../internal/version.Version=...".
var Version = "dev"
