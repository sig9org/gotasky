package selfupdate

import (
	"context"
	"errors"
	"testing"
)

// Non-semver current versions (e.g. "dev", used for local builds) must be
// rejected before any network call, since the underlying library panics on
// an invalid version string instead of returning an error.
func TestUpdate_NonSemverVersionReturnsErrNotAReleaseBuild(t *testing.T) {
	_, err := Update(context.Background(), "sig9org/gotasky", "dev")
	if !errors.Is(err, ErrNotAReleaseBuild) {
		t.Fatalf("Update() error = %v, want ErrNotAReleaseBuild", err)
	}
}
