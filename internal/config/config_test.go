package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ParsesFilesSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
files:
  default:
    path: Taskfile.yml
    version: [default.txt]
    includes: [other.txt]
    vars: [cleanup.txt]
    tasks: [default.txt, cleanup.txt]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	f, ok := cfg.Files["default"]
	if !ok {
		t.Fatal(`expected "default" file entry`)
	}
	if f.Path != "Taskfile.yml" {
		t.Errorf("Path = %q, want Taskfile.yml", f.Path)
	}
	if len(f.Version) != 1 || f.Version[0] != "default.txt" {
		t.Errorf("Version = %v", f.Version)
	}
	if len(f.Includes) != 1 || f.Includes[0] != "other.txt" {
		t.Errorf("Includes = %v", f.Includes)
	}
	if len(f.Tasks) != 2 {
		t.Errorf("Tasks = %v, want 2 entries", f.Tasks)
	}
}

func TestLoad_ParsesIgnoreSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
ignore:
  - .git
  - .gitkeep
files:
  default:
    path: Taskfile.yml
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Ignore) != 2 || cfg.Ignore[0] != ".git" || cfg.Ignore[1] != ".gitkeep" {
		t.Errorf("Ignore = %v, want [.git .gitkeep]", cfg.Ignore)
	}
}

func TestLoad_OmittedIgnoreIsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
files:
  default:
    path: Taskfile.yml
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Ignore) != 0 {
		t.Errorf("Ignore = %v, want empty", cfg.Ignore)
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected error for missing config file")
	}
}

// A config file cannot name its own path from inside itself -- that's
// contradictory, since the path must already be resolved before the file
// can be read. A top-level "config" key must be rejected explicitly rather
// than silently ignored.
func TestLoad_RejectsTopLevelConfigKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
config: other.yml
files:
  default:
    path: Taskfile.yml
    version: [default.txt]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for a top-level \"config\" key")
	}
}

func TestLoad_InvalidYAMLReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	if err := os.WriteFile(path, []byte("files: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid YAML config")
	}
}

// A file can reference more than one preset; each preset's snippets are merged
// in, ahead of the file's own directly-listed snippets, in the order the
// presets are named.
func TestLoad_ResolvesMultiplePresets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
presets:
  common:
    version: [default.txt]
    vars: [common.txt]
    tasks: [common.txt]
  extra:
    includes: [extra.txt]
    tasks: [extra.txt]

files:
  default:
    path: Taskfile.yml
    presets: [common, extra]
    tasks: [own.txt]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	f, ok := cfg.Files["default"]
	if !ok {
		t.Fatal(`expected "default" file entry`)
	}
	if got, want := f.Version, []string{"default.txt"}; !equalSlices(got, want) {
		t.Errorf("Version = %v, want %v", got, want)
	}
	if got, want := f.Includes, []string{"extra.txt"}; !equalSlices(got, want) {
		t.Errorf("Includes = %v, want %v", got, want)
	}
	if got, want := f.Vars, []string{"common.txt"}; !equalSlices(got, want) {
		t.Errorf("Vars = %v, want %v", got, want)
	}
	if got, want := f.Tasks, []string{"common.txt", "extra.txt", "own.txt"}; !equalSlices(got, want) {
		t.Errorf("Tasks = %v, want %v", got, want)
	}
}

func TestLoad_UnknownPresetReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
files:
  default:
    path: Taskfile.yml
    presets: [missing]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown preset reference")
	}
}

func TestLoad_LegacySetsKeyReturnsRenameError(t *testing.T) {
	for name, content := range map[string]string{
		"top-level": "sets:\n  common:\n    tasks: [common.yml]\nfiles: {}\n",
		"file":      "presets:\n  common:\n    tasks: [common.yml]\nfiles:\n  default:\n    path: Taskfile.yml\n    sets: [common]\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), `renamed to "presets"`) {
				t.Fatalf("Load() error = %v, want presets rename guidance", err)
			}
		})
	}
}

func TestLoad_RemovedHeadersKeyReturnsVersionGuidance(t *testing.T) {
	for name, content := range map[string]string{
		"file":   "files:\n  default:\n    path: Taskfile.yml\n    headers: [default.yml]\n",
		"preset": "presets:\n  common:\n    headers: [default.yml]\nfiles: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), `use the Root Schema property "version"`) {
				t.Fatalf("Load() error = %v, want version migration guidance", err)
			}
		})
	}
}

func TestLoad_DistinguishesPresetsFromRootSetProperty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	content := `
presets:
  shell-safe:
    set: [safe.yml]
files:
  default:
    path: Taskfile.yml
    presets: [shell-safe]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Files["default"].Set, []string{"safe.yml"}; !equalSlices(got, want) {
		t.Fatalf("Set = %v, want %v", got, want)
	}
}

func TestLoad_IdenticalReferencedPresetsWarnAndMergeOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
presets:
  common:
    version: [v3.yml]
    env: [common.yml]
  duplicate:
    version: [v3.yml]
    env: [common.yml]
files:
  default:
    path: Taskfile.yml
    presets: [common, duplicate, common]
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	f := cfg.Files["default"]
	if got, want := f.Version, []string{"v3.yml"}; !equalSlices(got, want) {
		t.Errorf("Version = %v, want %v", got, want)
	}
	if got, want := f.Env, []string{"common.yml"}; !equalSlices(got, want) {
		t.Errorf("Env = %v, want %v", got, want)
	}
	if len(f.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want two duplicate-preset warnings", f.Warnings)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
