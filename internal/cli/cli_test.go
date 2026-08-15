package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sig9org/gotasky/internal/version"
)

func TestRun_VersionFlag(t *testing.T) {
	old := version.Version
	version.Version = "v1.2.3"
	defer func() { version.Version = old }()

	for _, opt := range []string{"--version", "-v"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{opt}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %s", opt, code, stderr.String())
		}
		want := version.String() + "\n"
		if stdout.String() != want {
			t.Errorf("%s: stdout = %q, want %q", opt, stdout.String(), want)
		}
		for _, part := range []string{"gotasky", "v1.2.3"} {
			if !strings.Contains(stdout.String(), part) {
				t.Errorf("%s: expected stdout to contain %q, got: %q", opt, part, stdout.String())
			}
		}
	}
}

// -u, -s, and -d were removed as shorthands for -update, -silent, and
// -dryrun: only the long forms remain, so each single-letter form must now
// be rejected as an unknown flag.
func TestRun_RemovedShorthandFlagsAreUnrecognized(t *testing.T) {
	for _, opt := range []string{"-u", "-s", "-d"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{opt}, &stdout, &stderr)
		if code != 2 {
			t.Errorf("%s: exit code = %d, want 2", opt, code)
		}
		if stderr.Len() == 0 {
			t.Errorf("%s: expected an error message on stderr", opt)
		}
	}
}

func TestRun_UnknownFlagReturnsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--bogus"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if stderr.Len() == 0 {
		t.Error("expected an error message on stderr")
	}
}

// -h and --help must both print usage (naming the tool and its version, and
// documenting the -v shorthand) and exit successfully, unlike a genuine
// usage error. -update, -dryrun, and -silent no longer have shorthands.
func TestRun_HelpFlag(t *testing.T) {
	old := version.Version
	version.Version = "v1.2.3"
	defer func() { version.Version = old }()

	for _, opt := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{opt}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, want 0", opt, code)
		}

		out := stderr.String()
		for _, want := range []string{"gotasky v1.2.3", "-update", "-v, -version", "-h, -help", "-c, -config", "-dryrun", "-debug", "-silent"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: help output missing %q, got:\n%s", opt, want, out)
			}
		}
		for _, notWant := range []string{"-u, -update", "-d, -dryrun", "-s, -silent"} {
			if strings.Contains(out, notWant) {
				t.Errorf("%s: help output still documents a removed shorthand %q, got:\n%s", opt, notWant, out)
			}
		}
	}
}

// writeFlagDocs must align the short option, long option, and description
// into fixed columns across every flag line -- a flag with no short option,
// and a long option much wider than the rest, must not throw off where
// every row's description starts.
func TestWriteFlagDocs_AlignsColumns(t *testing.T) {
	docs := []flagDoc{
		{"a", "alpha", "short desc"},
		{"", "muchlongerflag", "another desc"},
		{"z", "zz", "zzz desc"},
	}
	var buf bytes.Buffer
	writeFlagDocs(&buf, docs)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(docs) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(docs), buf.String())
	}

	descCol := strings.Index(lines[0], docs[0].desc)
	if descCol < 0 {
		t.Fatalf("description %q not found in line %q", docs[0].desc, lines[0])
	}
	for i, line := range lines {
		if idx := strings.Index(line, docs[i].desc); idx != descCol {
			t.Errorf("line %d: description starts at column %d, want %d (line: %q)", i, idx, descCol, line)
		}
	}
}

// With no flags and no overrides, generate must read ./config.yml and
// ./templates, matching the documented defaults.
func TestRun_GenerateUsesDefaultPaths(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); err != nil {
		t.Fatalf("expected generated Taskfile.yml: %v", err)
	}
}

// When ./config.yml is absent, generate must fall back to ./config.yaml.
func TestRun_GenerateFallsBackToConfigYaml(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yaml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); err != nil {
		t.Fatalf("expected generated Taskfile.yml: %v", err)
	}
}

// config.yml takes priority over config.yaml when both are present.
func TestRun_GenerateConfigYmlTakesPriorityOverConfigYaml(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "from-yml.yml")
	writeConfig(t, filepath.Join(dir, "config.yaml"), "from-yaml.yml")

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "from-yml.yml")); err != nil {
		t.Fatalf("expected config.yml to take priority: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "from-yaml.yml")); err == nil {
		t.Fatal("config.yaml should have been ignored in favor of config.yml")
	}
}

// -c and -config must both let the user point at a config file outside the
// current directory, taking priority over the config.yml/config.yaml
// defaults.
func TestRun_ConfigFlag(t *testing.T) {
	for _, opt := range []string{"-c", "-config"} {
		dir := t.TempDir()
		t.Chdir(dir)

		writeTemplateFixtures(t, filepath.Join(dir, "templates"))
		configPath := filepath.Join(dir, "custom-config.yml")
		writeConfig(t, configPath, "Taskfile.yml")

		var stdout, stderr bytes.Buffer
		code := Run([]string{opt, configPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %s", opt, code, stderr.String())
		}

		if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); err != nil {
			t.Fatalf("%s: expected generated Taskfile.yml: %v", opt, err)
		}
	}
}

// -dryrun must validate generation without writing any files, while still
// reporting [OK]/[NG] and returning a non-zero exit code on failure.
func TestRun_DryRunFlagSkipsWriting(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-dryrun"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[OK]") {
		t.Errorf("expected [OK] in stdout, got: %s", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected no file to be written, stat err = %v", err)
	}
}

// A failing generation must have its "[NG] ..." status line wrapped in the
// ANSI red color codes, so it stands out in a terminal.
func TestRun_GenerateFailureIsColoredRed(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    version: [missing.txt]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a failing generation")
	}

	want := "\x1b[31m[NG] broken: Taskfile.yml\x1b[0m\n"
	if !strings.Contains(stderr.String(), want) {
		t.Errorf("expected red-colored [NG] line %q, got: %q", want, stderr.String())
	}
}

// Two config entries writing the same output path in one run must print an
// orange-colored [WARN] line, without turning generation itself into a
// failure.
func TestRun_DuplicateOutputPathWarnsInOrange(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	content := "files:\n" +
		"  a-first:\n" +
		"    path: Taskfile.yml\n" +
		"    version: [default.txt]\n" +
		"  b-second:\n" +
		"    path: Taskfile.yml\n" +
		"    version: [default.txt]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, stderr = %s", code, stderr.String())
	}

	if !strings.Contains(stderr.String(), ansiOrange+"[WARN]") {
		t.Errorf("expected an orange-colored [WARN] line, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "b-second") || !strings.Contains(stderr.String(), "Taskfile.yml") {
		t.Errorf("expected the warning to name the duplicate file and path, got: %q", stderr.String())
	}
}

// Dry run must still surface generation failures with a non-zero exit code.
func TestRun_DryRunReportsFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    version: [missing.txt]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-dryrun"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a failing dry run")
	}
	if !strings.Contains(stderr.String(), "[NG]") {
		t.Errorf("expected [NG] in stderr, got: %s", stderr.String())
	}
}

// -debug must print additional [DEBUG] diagnostics, gray-colored and
// timestamped, to stdout (not stderr) without changing the outcome of
// generation.
func TestRun_DebugFlagPrintsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ansiGray+"[DEBUG]") {
		t.Errorf("expected gray-colored [DEBUG] output in stdout, got: %s", stdout.String())
	}
	if strings.Contains(stderr.String(), "[DEBUG]") {
		t.Errorf("expected no [DEBUG] output in stderr, got: %s", stderr.String())
	}
}

// -debug must show the resolved Config.Ignore list, since it's part of how
// templatesDirs gets scanned for the duplicate-filename check.
func TestRun_DebugFlagPrintsIgnoreList(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	content := "ignore:\n  - .DS_Store\n  - .gitkeep\n" +
		"files:\n" +
		"  default:\n" +
		"    path: Taskfile.yml\n" +
		"    version: [default.txt]\n" +
		"    vars: [sample.txt]\n" +
		"    tasks: [sample.txt]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ignore: [.DS_Store .gitkeep]") {
		t.Errorf("expected debug output to list ignored filenames, got: %s", stdout.String())
	}
}

// -debug on a file that references a preset must name the preset and show what
// that preset individually contributed per category, not just the file's
// final merged lists.
func TestRun_DebugFlagPrintsPresetContributions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	content := "presets:\n" +
		"  common:\n" +
		"    version: [default.txt]\n" +
		"    vars: [sample.txt]\n" +
		"files:\n" +
		"  default:\n" +
		"    path: Taskfile.yml\n" +
		"    presets: [common]\n" +
		"    tasks: [sample.txt]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-debug"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "presets: [common]") {
		t.Errorf("expected debug output to name the referenced preset, got: %s", out)
	}
	if !strings.Contains(out, "common -> version: [default.txt], includes: [], vars: [sample.txt], tasks: []") {
		t.Errorf("expected debug output to show the preset's own per-category contribution, got: %s", out)
	}
}

// -silent must suppress all stdout output, while still writing files and
// reporting errors on stderr as usual.
func TestRun_SilentFlagSuppressesStdout(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-silent"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout output, got: %q", stdout.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); err != nil {
		t.Fatalf("expected generated Taskfile.yml: %v", err)
	}
}

// Silent mode must not swallow error reporting on stderr.
func TestRun_SilentFlagStillReportsFailureOnStderr(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    version: [missing.txt]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-silent"}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for a failing generation")
	}
	if stdout.Len() != 0 {
		t.Errorf("expected no stdout output, got: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "[NG]") {
		t.Errorf("expected [NG] in stderr, got: %s", stderr.String())
	}
}

// -debug takes priority over -silent: requesting both isn't a conflict to
// reject, but debug output must still appear.
func TestRun_DebugFlagOverridesSilent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

	var stdout, stderr bytes.Buffer
	code := Run([]string{"-debug", "-silent"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[DEBUG]") {
		t.Errorf("expected [DEBUG] output in stdout, got: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[OK]") {
		t.Errorf("expected -debug to override -silent and still print [OK] to stdout, got: %q", stdout.String())
	}
}

// The config file's top-level "templates" key must take priority over the
// built-in "templates" directory default.
func TestRun_GenerateHonorsTemplatesKeyOverride(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	templatesDir := filepath.Join(dir, "tpl")
	writeTemplateFixtures(t, templatesDir)

	content := fmt.Sprintf(
		"templates: [%q]\nfiles:\n  default:\n    path: %s\n    version: [default.txt]\n    vars: [sample.txt]\n    tasks: [sample.txt]\n",
		templatesDir, filepath.ToSlash(filepath.Join("out", "Taskfile.yml")),
	)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "out", "Taskfile.yml")); err != nil {
		t.Fatalf("expected generated Taskfile.yml using the overridden templates dir: %v", err)
	}
}

// The config file's "templates" key accepts multiple directories, searched
// in order -- letting snippets be spread across more than one location
// (e.g. a project-specific directory layered on top of a shared one).
func TestRun_GenerateHonorsMultipleTemplatesDirs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	sharedDir := filepath.Join(dir, "shared-templates")
	writeTemplateFixtures(t, sharedDir)

	// project-templates supplies a snippet under a name not present in
	// shared-templates, so there's no filename overlap between the two
	// dirs — only a genuinely missing category (version, tasks) falls back
	// to shared-templates.
	projectDir := filepath.Join(dir, "project-templates")
	if err := os.MkdirAll(filepath.Join(projectDir, "vars"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "vars", "only-here.txt"), []byte("  ONLY_HERE:\n    - x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	content := fmt.Sprintf(
		"templates: [%q, %q]\nfiles:\n  default:\n    path: Taskfile.yml\n    version: [default.txt]\n    vars: [only-here.txt]\n    tasks: [sample.txt]\n",
		projectDir, sharedDir,
	)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "Taskfile.yml"))
	if err != nil {
		t.Fatalf("expected generated Taskfile.yml: %v", err)
	}
	if !strings.Contains(string(data), "ONLY_HERE") {
		t.Errorf("expected project-templates' vars/only-here.txt to be used, got:\n%s", data)
	}
	if !strings.Contains(string(data), "version:") {
		t.Errorf("expected version/default.txt to fall back to shared-templates, got:\n%s", data)
	}
}

// A filename present in more than one configured templates directory is a
// configuration mistake, not a "first directory wins" search-path case —
// generation must fail before writing anything.
func TestRun_GenerateFailsOnDuplicateTemplateFilenameAcrossDirs(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	firstDir := filepath.Join(dir, "first-templates")
	writeTemplateFixtures(t, firstDir)

	secondDir := filepath.Join(dir, "second-templates")
	writeTemplateFixtures(t, secondDir)

	content := fmt.Sprintf(
		"templates: [%q, %q]\nfiles:\n  default:\n    path: Taskfile.yml\n    version: [default.txt]\n    vars: [sample.txt]\n    tasks: [sample.txt]\n",
		firstDir, secondDir,
	)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "[ERROR]Duplicate template filenames") {
		t.Errorf("expected stderr to mention duplicate template filenames, got: %q", stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); !os.IsNotExist(err) {
		t.Errorf("expected no output file to be written when duplicate templates are detected, stat err = %v", err)
	}
}

func TestRun_GenerateMissingConfigReturnsError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

// "dev" (the default for local, non-released builds) isn't a valid semantic
// version, so --update must fail cleanly rather than hit the network or panic.
func TestRun_UpdateOnNonReleaseBuildFailsCleanly(t *testing.T) {
	old := version.Version
	version.Version = "dev"
	defer func() { version.Version = old }()

	var stdout, stderr bytes.Buffer
	code := Run([]string{"--update"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1, stderr = %s", code, stderr.String())
	}
}

func TestRun_IdenticalPresetsPrintWarningWithoutFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	content := "presets:\n" +
		"  first:\n    version: [default.txt]\n" +
		"  second:\n    version: [default.txt]\n" +
		"files:\n  default:\n    path: Taskfile.yml\n    presets: [first, second]\n    tasks: [sample.txt]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "[WARN]") || !strings.Contains(stderr.String(), "exactly the same elements") {
		t.Errorf("expected duplicate-preset warning, got: %q", stderr.String())
	}
}

func writeConfig(t *testing.T, path, outputPath string) {
	t.Helper()
	content := "files:\n" +
		"  default:\n" +
		"    path: " + outputPath + "\n" +
		"    version: [default.txt]\n" +
		"    vars: [sample.txt]\n" +
		"    tasks: [sample.txt]\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTemplateFixtures(t *testing.T, dir string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("version/default.txt", "version: '3'\n")
	mustWrite("vars/sample.txt", "  ZEBRA:\n    - z\n\n  APPLE:\n    - a\n")
	mustWrite("tasks/sample.txt", "  zeta:\n    cmds:\n      - echo zeta\n\n  default:\n    cmds:\n      - task --list\n\n  alpha:\n    cmds:\n      - echo alpha\n")
}
