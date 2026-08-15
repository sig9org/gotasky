package version

import "testing"

func TestStringContainsOnlyTaggedVersion(t *testing.T) {
	old := Version
	Version = "v0.0.4"
	t.Cleanup(func() { Version = old })

	if got, want := String(), "gotasky v0.0.4"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
