// Package selfupdate checks GitHub releases for a newer gotasky build and,
// if one is found, replaces the currently running executable with it.
package selfupdate

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	selfupdatelib "github.com/creativeprojects/go-selfupdate"
)

// ErrUpToDate is returned by Update when currentVersion is already the
// latest available release.
var ErrUpToDate = errors.New("already up to date")

// ErrNotAReleaseBuild is returned by Update when currentVersion is not a
// valid semantic version (e.g. the "dev" placeholder used for local
// builds), since there is nothing meaningful to compare against.
var ErrNotAReleaseBuild = errors.New("current build is not a versioned release")

// Update checks repoSlug (e.g. "sig9org/gotasky") for a release newer than
// currentVersion and, if found, downloads it and replaces the running
// executable in place. It returns ErrUpToDate if no update is needed, or
// ErrNotAReleaseBuild if currentVersion isn't a valid semantic version.
func Update(ctx context.Context, repoSlug, currentVersion string) (string, error) {
	if _, err := semver.NewVersion(currentVersion); err != nil {
		return "", ErrNotAReleaseBuild
	}

	repo := selfupdatelib.ParseSlug(repoSlug)

	latest, found, err := selfupdatelib.DetectLatest(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("detect latest release: %w", err)
	}
	if !found {
		return "", fmt.Errorf("no release found for this platform in %s", repoSlug)
	}

	if latest.LessOrEqual(currentVersion) {
		return latest.Version(), ErrUpToDate
	}

	if _, err := selfupdatelib.UpdateSelf(ctx, currentVersion, repo); err != nil {
		return "", fmt.Errorf("update to %s: %w", latest.Version(), err)
	}

	return latest.Version(), nil
}
