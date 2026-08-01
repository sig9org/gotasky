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

	"github.com/sig9org/gotasky/internal/config"
	"github.com/sig9org/gotasky/internal/generator"
	"github.com/sig9org/gotasky/internal/selfupdate"
	"github.com/sig9org/gotasky/internal/version"
)

const (
	// defaultTemplatesDir is used unless overridden by the TEMPLATES
	// variable in .env (loaded by main before Run is called) or the
	// process environment.
	defaultTemplatesDir = "templates"

	// repoSlug is the GitHub repository that publishes gotasky release binaries.
	repoSlug = "sig9org/gotasky"

	// ansiRed/ansiYellow/ansiReset color the "[NG]"/"[WARN]" status lines so
	// failures and warnings stand out.
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiReset  = "\x1b[0m"
)

// defaultConfigNames are the config file names looked for, in order, in the
// current directory when CONFIG isn't set.
var defaultConfigNames = []string{"config.yml", "config.yaml"}

// Run executes the CLI for the given arguments (typically os.Args[1:]) and
// returns the process exit code. Output is written to stdout/stderr.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gotasky", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var update, showVersion, debug, dryRun, silent bool
	var configFlag string
	fs.BoolVar(&update, "update", false, "update gotasky itself to the latest GitHub release")
	fs.BoolVar(&update, "u", false, "update gotasky itself to the latest GitHub release (shorthand)")
	fs.BoolVar(&showVersion, "version", false, "print the gotasky version")
	fs.BoolVar(&showVersion, "v", false, "print the gotasky version (shorthand)")
	fs.StringVar(&configFlag, "config", "", "path to the config file (default: CONFIG env var, or ./config.yml / ./config.yaml)")
	fs.StringVar(&configFlag, "c", "", "path to the config file (shorthand)")
	fs.BoolVar(&debug, "debug", false, "print detailed debug information")
	fs.BoolVar(&dryRun, "dryrun", false, "validate generation without writing any files")
	fs.BoolVar(&dryRun, "d", false, "validate generation without writing any files (shorthand)")
	fs.BoolVar(&silent, "silent", false, "suppress standard output (overridden by -debug)")
	fs.BoolVar(&silent, "s", false, "suppress standard output (shorthand, overridden by -debug)")

	fs.Usage = func() {
		out := fs.Output()
		fmt.Fprintf(out, "gotasky %s\n\n", version.Version)
		fmt.Fprintln(out, "Usage of gotasky:")
		fmt.Fprintln(out, "  -c, -config <path>")
		fmt.Fprintln(out, "        path to the config file (default: CONFIG env var, or ./config.yml / ./config.yaml)")
		fmt.Fprintln(out, "  -d, -dryrun")
		fmt.Fprintln(out, "        validate generation without writing any files")
		fmt.Fprintln(out, "  -debug")
		fmt.Fprintln(out, "        print detailed debug information")
		fmt.Fprintln(out, "  -s, -silent")
		fmt.Fprintln(out, "        suppress standard output (overridden by -debug)")
		fmt.Fprintln(out, "  -u, -update")
		fmt.Fprintln(out, "        update gotasky itself to the latest GitHub release")
		fmt.Fprintln(out, "  -v, -version")
		fmt.Fprintln(out, "        print the gotasky version")
		fmt.Fprintln(out, "  -h, --help")
		fmt.Fprintln(out, "        show this help message")
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
		fmt.Fprintln(stdout, version.Version)
		return 0
	case update:
		return runUpdate(stdout, stderr)
	default:
		return runGenerate(stdout, stderr, configFlag, debug, dryRun, silent)
	}
}

func runGenerate(stdout, stderr io.Writer, configFlag string, debug, dryRun, silent bool) int {
	configPath := configPathOrDefault(configFlag)
	templatesDir := envOr("TEMPLATES", defaultTemplatesDir)

	if debug {
		fmt.Fprintf(stderr, "[DEBUG] config file: %s\n", configPath)
		fmt.Fprintf(stderr, "[DEBUG] templates dir: %s\n", templatesDir)
		fmt.Fprintf(stderr, "[DEBUG] dry run: %v\n", dryRun)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if debug {
		debugLogConfig(stderr, cfg)
	}

	results, err := generator.GenerateAll(cfg, templatesDir, ".", dryRun)
	for _, result := range results {
		if result.Warning != "" {
			fmt.Fprintf(stderr, "%s[WARN] %s: %s%s\n", ansiYellow, result.Name, result.Warning, ansiReset)
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

// debugLogConfig writes the parsed config's file entries to out, one file
// per line block, in a stable name-sorted order.
func debugLogConfig(out io.Writer, cfg *config.Config) {
	names := make([]string, 0, len(cfg.Files))
	for name := range cfg.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fileCfg := cfg.Files[name]
		fmt.Fprintf(out, "[DEBUG] %s -> %s\n", name, fileCfg.Path)
		fmt.Fprintf(out, "[DEBUG]   headers:  %v\n", fileCfg.Headers)
		fmt.Fprintf(out, "[DEBUG]   includes: %v\n", fileCfg.Includes)
		fmt.Fprintf(out, "[DEBUG]   vars:     %v\n", fileCfg.Vars)
		fmt.Fprintf(out, "[DEBUG]   tasks:    %v\n", fileCfg.Tasks)
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

// envOr reads key from the process environment (populated from .env by
// main, when present), falling back to fallback when unset or empty. This
// is how .env values take priority over the built-in defaults.
func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// configPathOrDefault returns flagValue (the -c/-config flag) when set,
// otherwise the CONFIG environment variable when set, otherwise the first of
// defaultConfigNames that exists in the current directory, otherwise
// defaultConfigNames[0] (so a missing-config error names a concrete file).
func configPathOrDefault(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if v, ok := os.LookupEnv("CONFIG"); ok && v != "" {
		return v
	}
	for _, name := range defaultConfigNames {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return defaultConfigNames[0]
}
