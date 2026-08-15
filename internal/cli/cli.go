// Package cli implements gotasky's command line: generating a Taskfile.yml
// from templates (the default action), or self-updating via --update.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sig9org/gotasky/internal/config"
	"github.com/sig9org/gotasky/internal/generator"
	"github.com/sig9org/gotasky/internal/selfupdate"
	"github.com/sig9org/gotasky/internal/version"
)

const (
	// defaultTemplatesDir is used unless overridden by the top-level
	// "templates" key in the config file.
	defaultTemplatesDir = "templates"

	// repoSlug is the GitHub repository that publishes gotasky release binaries.
	repoSlug = "sig9org/gotasky"

	// ansiRed/ansiOrange/ansiGray/ansiReset color, respectively, error,
	// warning, and debug output so each stands out from normal (uncolored)
	// messages.
	ansiRed    = "\x1b[31m"
	ansiOrange = "\x1b[38;5;208m"
	ansiGray   = "\x1b[90m"
	ansiReset  = "\x1b[0m"

	// debugTimestampFormat is used to prefix each [DEBUG] line.
	debugTimestampFormat = "2006-01-02T15:04:05.000"
)

// defaultConfigNames are the config file names looked for, in order, in the
// current directory when -c/-config isn't set.
var defaultConfigNames = []string{"config.yml", "config.yaml"}

// flagDoc documents one flag for the aligned help output printed by
// fs.Usage: an optional short form, its long form, and a description.
type flagDoc struct {
	short string
	long  string
	desc  string
}

// flagDocs lists every flag in the order shown by -h/-help.
var flagDocs = []flagDoc{
	{"c", "config <path>", "path to the config file"},
	{"", "dryrun", "validate generation without writing any files"},
	{"", "debug", "print detailed debug information"},
	{"", "silent", "suppress standard output (overridden by -debug)"},
	{"", "update", "update gotasky itself to the latest GitHub release"},
	{"v", "version", "print the gotasky version"},
	{"h", "help", "show this help message"},
}

// writeFlagDocs prints docs as one aligned line per flag: a fixed-width
// short-form column (blank when a flag has no shorthand), a long-form
// column padded to the widest entry, then the description -- so the short
// option, long option, and description all line up across every row.
func writeFlagDocs(out io.Writer, docs []flagDoc) {
	const shortWidth = 4 // e.g. "-c, " or 4 blank spaces
	const descGap = 2

	longWidth := 0
	for _, d := range docs {
		if n := len(d.long) + 1; n > longWidth { // +1 for the leading "-"
			longWidth = n
		}
	}

	for _, d := range docs {
		short := strings.Repeat(" ", shortWidth)
		if d.short != "" {
			short = fmt.Sprintf("-%s, ", d.short)
		}
		long := "-" + d.long
		fmt.Fprintf(out, "  %s%-*s%s%s\n", short, longWidth, long, strings.Repeat(" ", descGap), d.desc)
	}
}

// Run executes the CLI for the given arguments (typically os.Args[1:]) and
// returns the process exit code. Output is written to stdout/stderr.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gotasky", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var update, showVersion, debug, dryRun, silent bool
	var configFlag string
	fs.BoolVar(&update, "update", false, "update gotasky itself to the latest GitHub release")
	fs.BoolVar(&showVersion, "version", false, "print the gotasky version")
	fs.BoolVar(&showVersion, "v", false, "print the gotasky version (shorthand)")
	fs.StringVar(&configFlag, "config", "", "path to the config file")
	fs.StringVar(&configFlag, "c", "", "path to the config file (shorthand)")
	fs.BoolVar(&debug, "debug", false, "print detailed debug information")
	fs.BoolVar(&dryRun, "dryrun", false, "validate generation without writing any files")
	fs.BoolVar(&silent, "silent", false, "suppress standard output (overridden by -debug)")

	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "%s\n\n", version.String())
		fmt.Fprintln(out, "Usage of gotasky:")
		writeFlagDocs(out, flagDocs)
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	// -debug always wins over -silent: debugging needs visibility, so
	// requesting both isn't a conflict to reject, just a priority to apply.
	if debug {
		silent = false
	}

	switch {
	case showVersion:
		fmt.Fprintln(stdout, version.String())
		return 0
	case update:
		return runUpdate(stdout, stderr)
	default:
		return runGenerate(stdout, stderr, configFlag, debug, dryRun, silent)
	}
}

func runGenerate(stdout, stderr io.Writer, configFlag string, debug, dryRun, silent bool) int {
	configPath := configPathOrDefault(configFlag)

	if debug {
		debugf(stdout, "config file: %s", configPath)
		debugf(stdout, "dry run: %v", dryRun)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s%s%s\n", ansiRed, err, ansiReset)
		return 1
	}

	templatesDirs := templatesDirsOrDefault(cfg.Templates)

	if debug {
		debugf(stdout, "templates dirs: %v", templatesDirs)
		debugf(stdout, "ignore: %v", cfg.Ignore)
		debugLogConfig(stdout, cfg)
	}

	results, err := generator.GenerateAll(cfg, templatesDirs, ".", dryRun)
	if err != nil && len(results) == 0 {
		// A fatal error with no per-file results (e.g. duplicate template
		// filenames across templatesDirs) has nothing to report in the
		// [OK]/[NG] loop below, so it must be printed here instead.
		fmt.Fprintf(stderr, "%s[ERROR]%s%s\n", ansiRed, err, ansiReset)
		return 1
	}
	for _, result := range results {
		if result.Warning != "" {
			fmt.Fprintf(stderr, "%s[WARN] %s: %s%s\n", ansiOrange, result.Name, result.Warning, ansiReset)
		}
		switch {
		case result.Error != nil:
			fmt.Fprintf(stderr, "%s[NG] %s: %s%s\n%s\n", ansiRed, result.Name, result.Path, ansiReset, result.Error)
		case silent:
			// suppressed
		case dryRun:
			fmt.Fprintf(stdout, "[OK] %s: %s (dry run, not written)\n", result.Name, result.Path)
		default:
			fmt.Fprintf(stdout, "[OK] %s: %s\n", result.Name, result.Path)
		}
	}
	if err != nil {
		return 1
	}
	return 0
}

// debugf writes a single timestamped, gray-colored "[DEBUG]" line to out.
func debugf(out io.Writer, format string, args ...any) {
	fmt.Fprintf(out, "%s[DEBUG] %s %s%s\n", ansiGray, time.Now().Format(debugTimestampFormat), fmt.Sprintf(format, args...), ansiReset)
}

var debugRootCategories = []string{
	"version", "dotenv", "env", "includes", "interval", "method", "output",
	"run", "set", "shopt", "silent", "vars", "tasks",
}

// debugLogConfig writes the parsed config's file entries to out, one file
// per line block, in a stable name-sorted order. For a file that references
// one or more presets, it also names those presets and, per preset, exactly
// which Root Schema categories that preset contributed -- before printing
// the file's final, already-resolved (preset contents merged in) per-category
// lists -- so -debug can answer "which preset brought in which snippet?", not
// just "what's the final list?".
func debugLogConfig(out io.Writer, cfg *config.Config) {
	names := make([]string, 0, len(cfg.Files))
	for name := range cfg.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fileCfg := cfg.Files[name]
		debugf(out, "%s -> %s", name, fileCfg.Path)
		debugf(out, "  presets: %v", fileCfg.Presets)
		for _, presetName := range fileCfg.Presets {
			preset := cfg.Presets[presetName]
			debugf(out, "    preset %s:", presetName)
			debugf(out, "      presets: %v", preset.Presets)
			for _, category := range debugRootCategories {
				debugf(out, "      %s: %v", category, preset.TemplateNames(category))
			}
		}
		for _, category := range debugRootCategories {
			debugf(out, "  %s: %v", category, fileCfg.TemplateNames(category))
		}
	}
}

func runUpdate(stdout, stderr io.Writer) int {
	latest, err := selfupdate.Update(context.Background(), repoSlug, version.Version)
	switch {
	case errors.Is(err, selfupdate.ErrUpToDate):
		fmt.Fprintf(stdout, "already up to date (%s)\n", version.Version)
		return 0
	case errors.Is(err, selfupdate.ErrNotAReleaseBuild):
		fmt.Fprintf(stderr, "current build (%s) is not a versioned release; skipping self-update\n", version.Version)
		return 1
	case err != nil:
		fmt.Fprintln(stderr, err)
		return 1
	default:
		fmt.Fprintf(stdout, "updated to version %s\n", latest)
		return 0
	}
}

// configPathOrDefault returns flagValue (the -c/-config flag) when set,
// otherwise the first of defaultConfigNames that exists in the current
// directory, otherwise defaultConfigNames[0] (so a missing-config error
// names a concrete file).
func configPathOrDefault(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	for _, name := range defaultConfigNames {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return defaultConfigNames[0]
}

// templatesDirsOrDefault returns dirs (the config file's top-level
// "templates" key) when non-empty, otherwise the built-in single-directory
// default.
func templatesDirsOrDefault(dirs []string) []string {
	if len(dirs) > 0 {
		return dirs
	}
	return []string{defaultTemplatesDir}
}
