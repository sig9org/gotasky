package cli

import (
	"bytes"
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
		if stdout.String() != "v1.2.3\n" {
			t.Errorf("%s: stdout = %q, want %q", opt, stdout.String(), "v1.2.3\n")
		}
	}
}

func TestRun_UpdateShorthandFlag(t *testing.T) {
	old := version.Version
	version.Version = "dev"
	defer func() { version.Version = old }()

	var stdoutLong, stderrLong bytes.Buffer
	longCode := Run([]string{"--update"}, &stdoutLong, &stderrLong)

	var stdoutShort, stderrShort bytes.Buffer
	shortCode := Run([]string{"-u"}, &stdoutShort, &stderrShort)

	if longCode != shortCode {
		t.Fatalf("exit codes differ: --update=%d -u=%d", longCode, shortCode)
	}
	if stderrLong.String() != stderrShort.String() {
		t.Errorf("stderr differs: --update=%q -u=%q", stderrLong.String(), stderrShort.String())
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
// documenting -u/-v shorthands) and exit successfully, unlike a genuine
// usage error.
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
		for _, want := range []string{"gotasky v1.2.3", "-u, -update", "-v, -version", "-h, --help", "-c, -config", "-d, -dryrun", "-debug", "-s, -silent"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: help output missing %q, got:\n%s", opt, want, out)
			}
		}
	}
}

// With no flags and no .env overrides, generate must read ./config.yml and
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
// current directory, taking priority over the CONFIG env var and the
// config.yml/config.yaml defaults.
func TestRun_ConfigFlag(t *testing.T) {
	for _, opt := range []string{"-c", "-config"} {
		dir := t.TempDir()
		t.Chdir(dir)

		writeTemplateFixtures(t, filepath.Join(dir, "templates"))
		configPath := filepath.Join(dir, "custom-config.yml")
		writeConfig(t, configPath, "Taskfile.yml")
		t.Setenv("CONFIG", "should-be-overridden.yml")

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

// -d and -dryrun must validate generation without writing any files, while
// still reporting [OK]/[NG] and returning a non-zero exit code on failure.
func TestRun_DryRunFlagSkipsWriting(t *testing.T) {
	for _, opt := range []string{"-d", "-dryrun"} {
		dir := t.TempDir()
		t.Chdir(dir)

		writeTemplateFixtures(t, filepath.Join(dir, "templates"))
		writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

		var stdout, stderr bytes.Buffer
		code := Run([]string{opt}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %s", opt, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "[OK]") {
			t.Errorf("%s: expected [OK] in stdout, got: %s", opt, stdout.String())
		}

		if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); !os.IsNotExist(err) {
			t.Fatalf("%s: expected no file to be written, stat err = %v", opt, err)
		}
	}
}

// A failing generation must have its "[NG] ..." status line wrapped in the
// ANSI red color codes, so it stands out in a terminal.
func TestRun_GenerateFailureIsColoredRed(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    headers: [missing.txt]\n"), 0o644); err != nil {
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

// Two config entries writing the same output path in one run must print a
// yellow-colored [WARN] line, without turning generation itself into a
// failure.
func TestRun_DuplicateOutputPathWarnsInYellow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	writeTemplateFixtures(t, filepath.Join(dir, "templates"))
	content := "files:\n" +
		"  a-first:\n" +
		"    path: Taskfile.yml\n" +
		"    headers: [default.txt]\n" +
		"  b-second:\n" +
		"    path: Taskfile.yml\n" +
		"    headers: [default.txt]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0, stderr = %s", code, stderr.String())
	}

	if !strings.Contains(stderr.String(), ansiYellow+"[WARN]") {
		t.Errorf("expected a yellow-colored [WARN] line, got: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "b-second") || !strings.Contains(stderr.String(), "Taskfile.yml") {
		t.Errorf("expected the warning to name the duplicate file and path, got: %q", stderr.String())
	}
}

// Dry run must still surface generation failures with a non-zero exit code.
func TestRun_DryRunReportsFailure(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    headers: [missing.txt]\n"), 0o644); err != nil {
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

// -debug must print additional [DEBUG] diagnostics without changing the
// outcome of generation.
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
	if !strings.Contains(stderr.String(), "[DEBUG]") {
		t.Errorf("expected [DEBUG] output in stderr, got: %s", stderr.String())
	}
}

// -s and -silent must suppress all stdout output, while still writing files
// and reporting errors on stderr as usual.
func TestRun_SilentFlagSuppressesStdout(t *testing.T) {
	for _, opt := range []string{"-s", "-silent"} {
		dir := t.TempDir()
		t.Chdir(dir)

		writeTemplateFixtures(t, filepath.Join(dir, "templates"))
		writeConfig(t, filepath.Join(dir, "config.yml"), "Taskfile.yml")

		var stdout, stderr bytes.Buffer
		code := Run([]string{opt}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s: exit code = %d, stderr = %s", opt, code, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("%s: expected no stdout output, got: %q", opt, stdout.String())
		}

		if _, err := os.Stat(filepath.Join(dir, "Taskfile.yml")); err != nil {
			t.Fatalf("%s: expected generated Taskfile.yml: %v", opt, err)
		}
	}
}

// Silent mode must not swallow error reporting on stderr.
func TestRun_SilentFlagStillReportsFailureOnStderr(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("files:\n  broken:\n    path: Taskfile.yml\n    headers: [missing.txt]\n"), 0o644); err != nil {
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
	if !strings.Contains(stderr.String(), "[DEBUG]") {
		t.Errorf("expected [DEBUG] output in stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "[OK]") {
		t.Errorf("expected -debug to override -silent and still print [OK] to stdout, got: %q", stdout.String())
	}
}

// CONFIG / TEMPLATES environment variables (as populated from .env by main)
// must take priority over the built-in "config.yml" / "templates" defaults.
func TestRun_GenerateHonorsEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	templatesDir := filepath.Join(dir, "tpl")
	writeTemplateFixtures(t, templatesDir)

	configPath := filepath.Join(dir, "my-config.yml")
	writeConfig(t, configPath, filepath.Join("out", "Taskfile.yml"))

	t.Setenv("CONFIG", configPath)
	t.Setenv("TEMPLATES", templatesDir)

	var stdout, stderr bytes.Buffer
	code := Run(nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, stderr.String())
	}

	if _, err := os.Stat(filepath.Join(dir, "out", "Taskfile.yml")); err != nil {
		t.Fatalf("expected generated Taskfile.yml at env-overridden path: %v", err)
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

func writeConfig(t *testing.T, path, outputPath string) {
	t.Helper()
	content := "files:\n" +
		"  default:\n" +
		"    path: " + outputPath + "\n" +
		"    headers: [default.txt]\n" +
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
	mustWrite("headers/default.txt", "version: '3'\n\nsilent: true\n")
	mustWrite("vars/sample.txt", "  ZEBRA:\n    - z\n\n  APPLE:\n    - a\n")
	mustWrite("tasks/sample.txt", "  zeta:\n    cmds:\n      - echo zeta\n\n  default:\n    cmds:\n      - task --list\n\n  alpha:\n    cmds:\n      - echo alpha\n")
}
