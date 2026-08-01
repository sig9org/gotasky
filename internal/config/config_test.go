package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ParsesFilesSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yml")
	content := `
files:
  default:
    path: Taskfile.yml
    headers: [default.txt]
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
	if len(f.Headers) != 1 || f.Headers[0] != "default.txt" {
		t.Errorf("Headers = %v", f.Headers)
	}
	if len(f.Includes) != 1 || f.Includes[0] != "other.txt" {
		t.Errorf("Includes = %v", f.Includes)
	}
	if len(f.Tasks) != 2 {
		t.Errorf("Tasks = %v, want 2 entries", f.Tasks)
	}
}

func TestLoad_MissingFileReturnsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected error for missing config file")
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
