package selfupdate

import (
	"context"
	"errors"
	"testing"
)

// Local development builds must be rejected before any network call.
func TestUpdate_NonSemverVersionReturnsErrNotAReleaseBuild(t *testing.T) {
	_, err := Update(context.Background(), "sig9org/gotasky", "dev")
	if !errors.Is(err, ErrNotAReleaseBuild) {
		t.Fatalf("Update() error = %v, want ErrNotAReleaseBuild", err)
	}
}
