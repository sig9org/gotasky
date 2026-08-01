package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sig9org/gotasky/internal/cli"
)

// TestGenerate_RealTemplates exercises the generator against this project's
// real templates/ directory, to catch regressions the unit tests (which use
// isolated fixtures) wouldn't see. It also confirms that a TEMPLATES
// environment variable (as would be set via .env) overrides the built-in
// "templates" default, while the config file falls back to the default
// "config.yml" name.
func TestGenerate_RealTemplates(t *testing.T) {
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	templatesDir := filepath.Join(repoRoot, "templates")

	dir := t.TempDir()
	t.Chdir(dir)

	configContent := `
files:
  sample:
    path: Taskfile.generated.yml
    headers: [default.yml]
    includes: [default.yml]
    vars: [greeting.yml]
    tasks: [default.yml, hello.yml]
`
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TEMPLATES", templatesDir)

	var stdout, stderr bytes.Buffer
	code := cli.Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("generate failed: %s", stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "Taskfile.generated.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "includes:\n  default:") {
		t.Errorf("expected \"includes\" section with \"default\" entry, got:\n%s", content)
	}

	tasksIdx := strings.Index(content, "tasks:\n")
	if tasksIdx == -1 {
		t.Fatalf("no tasks section found:\n%s", content)
	}
	after := content[tasksIdx+len("tasks:\n"):]
	if !strings.HasPrefix(after, "  default:") {
		t.Errorf("expected \"default\" task pinned first right after \"tasks:\", got:\n%.80s", after)
	}
}
