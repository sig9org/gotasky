package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sig9org/gotasky/internal/config"
)

func TestSortEntries_AlphabeticalNoPinned(t *testing.T) {
	input := "  zebra:\n    - z\n\n  apple:\n    - a"
	got := sortEntries(input, "\n\n", nil)
	want := "  apple:\n    - a\n\n  zebra:\n    - z"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSortEntries_PinnedFirst(t *testing.T) {
	input := "  zeta:\n    cmds: []\n\n  default:\n    cmds: []\n\n  alpha:\n    cmds: []"
	got := sortEntries(input, "\n\n", []string{"default"})
	want := "  default:\n    cmds: []\n\n  alpha:\n    cmds: []\n\n  zeta:\n    cmds: []"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSortEntries_CaseInsensitive(t *testing.T) {
	input := "  Bravo:\n    - b\n\n  alpha:\n    - a"
	got := sortEntries(input, "\n\n", nil)
	want := "  alpha:\n    - a\n\n  Bravo:\n    - b"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSortEntries_StripsTrailingBlankLines(t *testing.T) {
	input := "  b:\n    - x\n\n\n  a:\n    - y\n\n"
	got := sortEntries(input, "\n\n", nil)
	want := "  a:\n    - y\n\n  b:\n    - x"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestSortEntries_PinnedNameAbsentIsNoop(t *testing.T) {
	input := "  zeta:\n    - z\n\n  alpha:\n    - a"
	got := sortEntries(input, "\n\n", []string{"default"})
	want := "  alpha:\n    - a\n\n  zeta:\n    - z"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuild_ProducesValidSortedYAML(t *testing.T) {
	g := New([]string{filepath.Join("testdata", "templates")})
	fileCfg := config.File{
		Headers:  []string{"default.txt"},
		Includes: []string{"sample.txt"},
		Vars:     []string{"sample.txt"},
		Tasks:    []string{"sample.txt"},
	}

	content, err := g.Build(fileCfg)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := ValidateYAML(content); err != nil {
		t.Fatalf("ValidateYAML() error = %v\n---\n%s", err, content)
	}

	includesAlphaIdx := strings.Index(content, "alpha: ./alpha")
	includesZetaIdx := strings.Index(content, "zeta: ./zeta")
	if includesAlphaIdx == -1 || includesZetaIdx == -1 || includesAlphaIdx > includesZetaIdx {
		t.Errorf("expected alpha before zeta in includes, got:\n%s", content)
	}

	includesIdx := strings.Index(content, "includes:")
	varsIdx := strings.Index(content, "vars:")
	if includesIdx == -1 || varsIdx == -1 || includesIdx > varsIdx {
		t.Errorf("expected includes section before vars section, got:\n%s", content)
	}

	appleIdx := strings.Index(content, "APPLE")
	zebraIdx := strings.Index(content, "ZEBRA")
	if appleIdx == -1 || zebraIdx == -1 || appleIdx > zebraIdx {
		t.Errorf("expected APPLE before ZEBRA in vars, got:\n%s", content)
	}

	tasksIdx := strings.Index(content, "tasks:\n")
	if tasksIdx == -1 {
		t.Fatalf("no tasks section found:\n%s", content)
	}
	tasksSection := content[tasksIdx:]
	defaultIdx := strings.Index(tasksSection, "default:")
	alphaIdx := strings.Index(tasksSection, "alpha:")
	zetaIdx := strings.Index(tasksSection, "zeta:")
	if !(defaultIdx != -1 && defaultIdx < alphaIdx && alphaIdx < zetaIdx) {
		t.Errorf("expected default before alpha before zeta in tasks, got:\n%s", tasksSection)
	}
}

// A snippet copy-pasted from a full Taskfile naturally starts with its own
// "tasks:" (or "vars:"/"includes:") line. Combining two such snippets for
// the same category must not duplicate that top-level key, which yaml.v3
// rejects as a "mapping key already defined" error.
func TestBuild_StripsDuplicateCategoryWrapperKey(t *testing.T) {
	g := New([]string{filepath.Join("testdata", "templates")})
	fileCfg := config.File{
		Headers: []string{"default.txt"},
		Tasks:   []string{"wrapped-a.txt", "wrapped-b.txt"},
	}

	content, err := g.Build(fileCfg)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := ValidateYAML(content); err != nil {
		t.Fatalf("ValidateYAML() error = %v\n---\n%s", err, content)
	}

	if n := strings.Count(content, "tasks:"); n != 1 {
		t.Errorf("expected exactly one \"tasks:\" key, got %d:\n%s", n, content)
	}
}

// A task definition under templates/tasks/ may declare its own "vars:"
// entry for variables only that task needs (e.g. GREETING used by
// "{{.GREETING}}"). It must be merged into the top-level vars: section
// rather than corrupting the tasks: section it was combined into.
func TestBuild_TaskSnippetCanDeclareOwnVars(t *testing.T) {
	g := New([]string{filepath.Join("testdata", "templates")})
	fileCfg := config.File{
		Headers: []string{"default.txt"},
		Tasks:   []string{"with-vars.txt"},
	}

	content, err := g.Build(fileCfg)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := ValidateYAML(content); err != nil {
		t.Fatalf("ValidateYAML() error = %v\n---\n%s", err, content)
	}

	if n := strings.Count(content, "vars:"); n != 1 {
		t.Errorf("expected exactly one \"vars:\" key, got %d:\n%s", n, content)
	}
	if n := strings.Count(content, "tasks:"); n != 1 {
		t.Errorf("expected exactly one \"tasks:\" key, got %d:\n%s", n, content)
	}
	if !strings.Contains(content, "GREETING: Hi there!") {
		t.Errorf("expected GREETING var to be present, got:\n%s", content)
	}
	if !strings.Contains(content, "greet:") {
		t.Errorf("expected greet task to be present, got:\n%s", content)
	}
}

func TestBuild_MissingTemplateReturnsError(t *testing.T) {
	g := New([]string{filepath.Join("testdata", "templates")})
	_, err := g.Build(config.File{Headers: []string{"missing.txt"}})
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}

// TemplatesDirs is a search path: a snippet missing from an earlier directory
// falls back to the next one, and an earlier directory wins when more than
// one contains a snippet of the same name.
func TestBuild_TemplatesDirsIsASearchPath(t *testing.T) {
	g := New([]string{t.TempDir(), filepath.Join("testdata", "templates")})
	content, err := g.Build(config.File{Headers: []string{"default.txt"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(content, "version:") {
		t.Errorf("expected fallback to the second directory's default.txt, got:\n%s", content)
	}

	override := t.TempDir()
	if err := os.MkdirAll(filepath.Join(override, "headers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(override, "headers", "default.txt"), []byte("overridden: true"), 0o644); err != nil {
		t.Fatal(err)
	}

	g = New([]string{override, filepath.Join("testdata", "templates")})
	content, err = g.Build(config.File{Headers: []string{"default.txt"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(content, "overridden: true") {
		t.Errorf("expected the first directory's default.txt to win, got:\n%s", content)
	}
}

func TestValidateYAML_RejectsInvalidYAML(t *testing.T) {
	err := ValidateYAML("foo: [1, 2")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateYAML_AcceptsValidYAML(t *testing.T) {
	if err := ValidateYAML("version: '3'\ntasks:\n  build:\n    cmds:\n      - echo hi\n"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateAll_WritesAndValidatesFiles(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"default": {
				Path:    "Taskfile.yml",
				Headers: []string{"default.txt"},
				Vars:    []string{"sample.txt"},
				Tasks:   []string{"sample.txt"},
			},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, false)
	if err != nil {
		t.Fatalf("GenerateAll() error = %v", err)
	}
	if len(results) != 1 || results[0].Error != nil {
		t.Fatalf("unexpected results: %+v", results)
	}

	data, err := os.ReadFile(filepath.Join(root, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	if err := ValidateYAML(string(data)); err != nil {
		t.Fatalf("generated file is not valid YAML: %v", err)
	}
}

func TestGenerateAll_DryRunDoesNotWriteFiles(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"default": {
				Path:    "Taskfile.yml",
				Headers: []string{"default.txt"},
				Vars:    []string{"sample.txt"},
				Tasks:   []string{"sample.txt"},
			},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, true)
	if err != nil {
		t.Fatalf("GenerateAll() error = %v", err)
	}
	if len(results) != 1 || results[0].Error != nil {
		t.Fatalf("unexpected results: %+v", results)
	}

	if _, err := os.Stat(filepath.Join(root, "Taskfile.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected no output file to be written in dry run, stat err = %v", err)
	}
}

func TestGenerateAll_DryRunReportsFailureForBadFile(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"broken": {
				Path:    "Taskfile.yml",
				Headers: []string{"missing.txt"},
			},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, true)
	if err == nil {
		t.Fatal("expected error from GenerateAll")
	}
	if len(results) != 1 || results[0].Error == nil {
		t.Fatalf("expected a failed result, got: %+v", results)
	}
}

func TestGenerateAll_ReportsFailureForBadFile(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"broken": {
				Path:    "Taskfile.yml",
				Headers: []string{"missing.txt"},
			},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, false)
	if err == nil {
		t.Fatal("expected error from GenerateAll")
	}
	if len(results) != 1 || results[0].Error == nil {
		t.Fatalf("expected a failed result, got: %+v", results)
	}
}

// Two files in the same run that write the same output Path must not fail
// generation, but the one processed second (in name-sorted order) must
// carry a Warning, since one silently overwrites the other's output.
func TestGenerateAll_WarnsOnDuplicateOutputPath(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"a-first": {
				Path:    "Taskfile.yml",
				Headers: []string{"default.txt"},
			},
			"b-second": {
				Path:    "Taskfile.yml",
				Headers: []string{"default.txt"},
			},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, false)
	if err != nil {
		t.Fatalf("GenerateAll() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got: %+v", results)
	}

	if results[0].Warning != "" {
		t.Errorf("expected no warning on the first writer, got: %q", results[0].Warning)
	}
	if results[1].Warning == "" {
		t.Error("expected a warning on the second writer of the same path")
	}
}

// Files that don't share an output Path must never carry a Warning.
func TestGenerateAll_NoWarningForDistinctPaths(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Files: map[string]config.File{
			"a": {Path: "a.yml", Headers: []string{"default.txt"}},
			"b": {Path: "b.yml", Headers: []string{"default.txt"}},
		},
	}

	results, err := GenerateAll(cfg, []string{filepath.Join("testdata", "templates")}, root, false)
	if err != nil {
		t.Fatalf("GenerateAll() error = %v", err)
	}
	for _, result := range results {
		if result.Warning != "" {
			t.Errorf("unexpected warning for %q: %q", result.Name, result.Warning)
		}
	}
}
