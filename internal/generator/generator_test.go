package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sig9org/gotasky/internal/config"
	"gopkg.in/yaml.v3"
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
		Version:  []string{"default.txt"},
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
		Version: []string{"default.txt"},
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
		Version: []string{"default.txt"},
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
	_, err := g.Build(config.File{Version: []string{"missing.txt"}})
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}

// TemplatesDirs is a search path: a snippet missing from an earlier directory
// falls back to the next one, and an earlier directory wins when more than
// one contains a snippet of the same name.
func TestBuild_TemplatesDirsIsASearchPath(t *testing.T) {
	g := New([]string{t.TempDir(), filepath.Join("testdata", "templates")})
	content, err := g.Build(config.File{Version: []string{"default.txt"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(content, "version:") {
		t.Errorf("expected fallback to the second directory's default.txt, got:\n%s", content)
	}

	override := t.TempDir()
	if err := os.MkdirAll(filepath.Join(override, "version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(override, "version", "default.txt"), []byte("version: overridden"), 0o644); err != nil {
		t.Fatal(err)
	}

	g = New([]string{override, filepath.Join("testdata", "templates")})
	content, err = g.Build(config.File{Version: []string{"default.txt"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(content, "version: overridden") {
		t.Errorf("expected the first directory's default.txt to win, got:\n%s", content)
	}
}

// A filename present in more than one templates directory, for any of the
// supported Root Schema categories, must fail generation up front rather than being
// silently resolved by "first directory wins" — regardless of whether any
// config.File actually references that filename.
func TestGenerateAll_FailsOnDuplicateTemplateFilenameAcrossDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	for _, dir := range []string{dirA, dirB} {
		if err := os.MkdirAll(filepath.Join(dir, "version"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "version", "default.txt"), []byte("version: '3'\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		Files: map[string]config.File{
			"default": {Path: "Taskfile.yml", Version: []string{"default.txt"}},
		},
	}

	root := t.TempDir()
	results, err := GenerateAll(cfg, []string{dirA, dirB}, root, false)
	if err == nil {
		t.Fatal("expected error from GenerateAll")
	}
	if results != nil {
		t.Errorf("expected no results when template dirs have duplicate filenames, got: %+v", results)
	}
	if !strings.Contains(err.Error(), "version/default.txt") {
		t.Errorf("expected error to name the duplicated file, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(root, "Taskfile.yml")); !os.IsNotExist(statErr) {
		t.Errorf("expected no output file to be written, stat err = %v", statErr)
	}
}

// A filename listed in config.Config.Ignore must be excluded from the
// duplicate check entirely, even though it's genuinely present in more than
// one templates directory -- this is how stray files like ".DS_Store" that
// an editor or OS drops into a templates directory are kept from being
// mistaken for a duplicated snippet.
func TestGenerateAll_IgnoresListedFilenamesInDuplicateCheck(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	for _, dir := range []string{dirA, dirB} {
		if err := os.MkdirAll(filepath.Join(dir, "version"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "version", ".DS_Store"), []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dirA, "version", "default.txt"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Ignore: []string{".DS_Store"},
		Files: map[string]config.File{
			"default": {Path: "Taskfile.yml", Version: []string{"default.txt"}},
		},
	}

	root := t.TempDir()
	results, err := GenerateAll(cfg, []string{dirA, dirB}, root, false)
	if err != nil {
		t.Fatalf("GenerateAll() error = %v, results = %+v", err, results)
	}
}

// Duplicate detection is per templates-directory pair, independent of
// whether a config.File actually references the duplicated snippet — an
// unused vars/ file duplicated across dirs must still be flagged.
func TestGenerateAll_FailsOnDuplicateEvenWhenUnreferenced(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	for _, dir := range []string{dirA, dirB} {
		if err := os.MkdirAll(filepath.Join(dir, "vars"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "vars", "unused.txt"), []byte("  X:\n    - 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dirA, "version"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "version", "default.txt"), []byte("version: '3'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Files: map[string]config.File{
			"default": {Path: "Taskfile.yml", Version: []string{"default.txt"}},
		},
	}

	_, err := GenerateAll(cfg, []string{dirA, dirB}, t.TempDir(), false)
	if err == nil {
		t.Fatal("expected error from GenerateAll for unreferenced duplicate")
	}
	if !strings.Contains(err.Error(), "vars/unused.txt") {
		t.Errorf("expected error to name the duplicated file, got: %v", err)
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
				Version: []string{"default.txt"},
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
				Version: []string{"default.txt"},
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
				Version: []string{"missing.txt"},
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
				Version: []string{"missing.txt"},
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
				Version: []string{"default.txt"},
			},
			"b-second": {
				Path:    "Taskfile.yml",
				Version: []string{"default.txt"},
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
			"a": {Path: "a.yml", Version: []string{"default.txt"}},
			"b": {Path: "b.yml", Version: []string{"default.txt"}},
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

func TestBuild_AllTaskfileRootSchemaDirectories(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"version/v3.yml":      "version: '3'\n",
		"output/group.yml":    "output: group\n",
		"method/checksum.yml": "method: checksum\n",
		"includes/common.yml": "  common: ./common.yml\n",
		"vars/app.yml":        "  APP: gotasky\n",
		"env/ci.yml":          "  CI: true\n",
		"tasks/default.yml":   "  default: echo ok\n",
		"silent/true.yml":     "silent: true\n",
		"dotenv/default.yml":  "dotenv: [.env]\n",
		"run/once.yml":        "run: once\n",
		"interval/watch.yml":  "interval: 1s\n",
		"set/safe.yml":        "set: [errexit, pipefail]\n",
		"shopt/globstar.yml":  "shopt: [globstar]\n",
	}
	for name, content := range fixtures {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	g := New([]string{root, filepath.Join(root, "does-not-exist")})
	content, err := g.Build(config.File{
		Version: []string{"v3.yml"}, Output: []string{"group.yml"}, Method: []string{"checksum.yml"},
		Includes: []string{"common.yml"}, Vars: []string{"app.yml"}, Env: []string{"ci.yml"}, Tasks: []string{"default.yml"},
		Silent: []string{"true.yml"}, Dotenv: []string{"default.yml"}, Run: []string{"once.yml"},
		Interval: []string{"watch.yml"}, Set: []string{"safe.yml"}, Shopt: []string{"globstar.yml"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := ValidateYAML(content); err != nil {
		t.Fatalf("Build() produced invalid YAML: %v\n%s", err, content)
	}
	for _, key := range rootCategories {
		if !strings.Contains(content, key+":") {
			t.Errorf("generated Taskfile does not contain root property %q:\n%s", key, content)
		}
	}
	previous := -1
	for _, key := range rootCategories {
		index := -1
		for offset, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, key+":") {
				index = offset
				break
			}
		}
		if index <= previous {
			t.Fatalf("root property %q is out of order:\n%s", key, content)
		}
		previous = index
	}
}

func TestBuild_SkipsUnconfiguredRootSchemaProperties(t *testing.T) {
	g := New([]string{filepath.Join("testdata", "templates")})
	content, err := g.Build(config.File{Version: []string{"default.txt"}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := content, "version: '3'\n"; got != want {
		t.Fatalf("Build() = %q, want %q", got, want)
	}
}

func TestBuild_MergesEveryDuplicateRootProperty(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string][2]string{
		"version":  {"version: '3'\n", "version: '3.1'\n"},
		"dotenv":   {"dotenv:\n  - .env\n", "dotenv:\n  - project.ini\n"},
		"env":      {"env:\n  FIRST: one\n", "env:\n  SECOND: two\n"},
		"includes": {"includes:\n  first: ./first.yml\n", "includes:\n  second: ./second.yml\n"},
		"interval": {"interval: 100ms\n", "interval: 1s\n"},
		"method":   {"method: checksum\n", "method: timestamp\n"},
		"output":   {"output:\n  group:\n    begin: begin\n", "output:\n  group:\n    end: end\n"},
		"run":      {"run: always\n", "run: once\n"},
		"set":      {"set:\n  - errexit\n", "set:\n  - pipefail\n"},
		"shopt":    {"shopt:\n  - globstar\n", "shopt:\n  - nullglob\n"},
		"silent":   {"silent: true\n", "silent: false\n"},
		"vars":     {"vars:\n  FIRST: one\n", "vars:\n  SECOND: two\n"},
		"tasks":    {"tasks:\n  default: echo default\n", "tasks:\n  build: echo build\n"},
	}
	for category, contents := range fixtures {
		categoryDir := filepath.Join(root, category)
		if err := os.MkdirAll(categoryDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for i, content := range contents {
			name := []string{"first.yml", "second.yml"}[i]
			if err := os.WriteFile(filepath.Join(categoryDir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	both := []string{"first.yml", "second.yml"}
	content, err := New([]string{root}).Build(config.File{
		Version: both, Dotenv: both, Env: both, Includes: both, Interval: both,
		Method: both, Output: both, Run: both, Set: both, Shopt: both,
		Silent: both, Vars: both, Tasks: both,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := ValidateYAML(content); err != nil {
		t.Fatalf("Build() produced duplicate or invalid YAML: %v\n%s", err, content)
	}

	var document yaml.Node
	if err := yaml.Unmarshal([]byte(content), &document); err != nil {
		t.Fatal(err)
	}
	rootMapping := document.Content[0]
	if got, want := len(rootMapping.Content)/2, len(rootCategories); got != want {
		t.Fatalf("top-level property count = %d, want %d:\n%s", got, want, content)
	}
	for _, category := range rootCategories {
		count := 0
		for i := 0; i+1 < len(rootMapping.Content); i += 2 {
			if rootMapping.Content[i].Value == category {
				count++
			}
		}
		if count != 1 {
			t.Errorf("top-level property %q appears %d times, want once", category, count)
		}
	}

	var decoded map[string]any
	if err := yaml.Unmarshal([]byte(content), &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded["version"]; got != "3.1" {
		t.Errorf("version = %#v, want last value 3.1", got)
	}
	if got := decoded["silent"]; got != false {
		t.Errorf("silent = %#v, want last value false", got)
	}
	for _, category := range []string{"dotenv", "set", "shopt"} {
		if got := len(decoded[category].([]any)); got != 2 {
			t.Errorf("%s length = %d, want 2", category, got)
		}
		if strings.Contains(content, category+":\n    -") || !strings.Contains(content, category+":\n  -") {
			t.Errorf("%s sequence does not use two-space indentation:\n%s", category, content)
		}
	}
	output := decoded["output"].(map[string]any)["group"].(map[string]any)
	if output["begin"] != "begin" || output["end"] != "end" {
		t.Errorf("output.group = %#v, want merged begin/end", output)
	}
	if !strings.Contains(content, "output:\n  group:\n    begin:") {
		t.Errorf("output mapping does not use two-space indentation per level:\n%s", content)
	}
}

func TestCheckNoDuplicateTemplatesScansEveryRootSchemaDirectory(t *testing.T) {
	for _, category := range rootCategories {
		t.Run(category, func(t *testing.T) {
			first, second := t.TempDir(), t.TempDir()
			for _, dir := range []string{first, second} {
				categoryDir := filepath.Join(dir, category)
				if err := os.MkdirAll(categoryDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(categoryDir, "duplicate.yml"), []byte("value\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := checkNoDuplicateTemplates([]string{first, filepath.Join(first, "missing"), second}, nil); err == nil {
				t.Fatalf("expected duplicate in %s directory to be detected", category)
			}
		})
	}
}
