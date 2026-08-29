// Package selfupdate checks GitHub releases for a newer gotasky build and,
// if one is found, replaces the currently running executable with it.
package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	selfupdatelib "github.com/sig9org/selfupdate-go"
)

// ErrUpToDate is returned by Update when currentVersion is already the
// latest available release.
var ErrUpToDate = errors.New("already up to date")

// ErrNotAReleaseBuild is returned by Update when currentVersion is empty or
// the "dev" placeholder used for local builds, since there is nothing
// meaningful to compare against.
var ErrNotAReleaseBuild = errors.New("current build is not a versioned release")

// Update checks repoSlug (e.g. "sig9org/gotasky") for a release newer than
// currentVersion and, if found, downloads it and replaces the running
// executable in place. It returns ErrUpToDate if no update is needed, or
// ErrNotAReleaseBuild if currentVersion isn't a release build version.
func Update(ctx context.Context, repoSlug, currentVersion string) (string, error) {
	if strings.TrimSpace(currentVersion) == "" || strings.EqualFold(strings.TrimSpace(currentVersion), "dev") {
		return "", ErrNotAReleaseBuild
	}

	updater, err := selfupdatelib.New(selfupdatelib.Config{
		Repository: repoSlug,
		Validator:  selfupdatelib.SHA256Validator{AssetName: "checksums.txt"},
	})
	if err != nil {
		return "", fmt.Errorf("configure self-update: %w", err)
	}

	result, err := updater.Update(ctx, currentVersion)
	if err != nil {
		return "", fmt.Errorf("self-update: %w", err)
	}
	if !result.Updated {
		return result.LatestVersion, ErrUpToDate
	}

	return result.LatestVersion, nil
}
