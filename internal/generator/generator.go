// Package generator combines template snippets (headers, includes, vars,
// tasks) into a complete Taskfile.yml, sorting includes, vars, and tasks and
// validating the result as YAML.
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sig9org/gotasky/internal/config"
)

// entryPattern matches the start of a top-level includes/vars/tasks entry —
// two spaces of indent, a non-space key, then a colon — whether the entry is
// a single line (e.g. "  build: ./build") or the first line of a block
// (e.g. "  build:").
var entryPattern = regexp.MustCompile(`^ {2}\S[^:\n]*:`)

// sectionCategories are the top-level Taskfile keys a template snippet may
// declare for itself, in addition to the category folder it's read from —
// e.g. a task definition under templates/tasks/ that also needs its own
// "vars:" entry.
var sectionCategories = []string{"vars", "tasks", "includes"}

// Generator renders Taskfile bodies from template snippets under TemplatesDir.
type Generator struct {
	TemplatesDir string
}

// New creates a Generator that reads template snippets from templatesDir.
func New(templatesDir string) *Generator {
	return &Generator{TemplatesDir: templatesDir}
}

func (g *Generator) readTemplate(category, name string) (string, error) {
	path := filepath.Join(g.TemplatesDir, category, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", path, err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}

func (g *Generator) renderAll(category string, names []string) ([]string, error) {
	blocks := make([]string, 0, len(names))
	for _, name := range names {
		block, err := g.readTemplate(category, name)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// collectSections reads every named template under category and splits each
// snippet's content into per-section chunks (see splitSections), merging the
// results into dest, keyed by section name ("vars", "tasks", or "includes").
func (g *Generator) collectSections(category string, names []string, dest map[string][]string) error {
	for _, name := range names {
		content, err := g.readTemplate(category, name)
		if err != nil {
			return err
		}
		for section, chunk := range splitSections(content, category) {
			dest[section] = append(dest[section], chunk)
		}
	}
	return nil
}

// splitSections splits a template snippet into per-section chunks. A line
// that is exactly "vars:", "tasks:", or "includes:" at column 0 switches the
// current section; content before the first such line (or all of it, if
// there is none) is attributed to defaultCategory, the folder the snippet
// was read from. This lets one snippet declare more than one top-level
// section — e.g. a task definition under templates/tasks/ that also needs
// its own "vars:" entry — without duplicating the key Build adds around
// each combined section.
func splitSections(content, defaultCategory string) map[string]string {
	lines := map[string][]string{}
	current := defaultCategory
	for _, line := range strings.Split(content, "\n") {
		if isSectionMarker(line) {
			current = strings.TrimSuffix(line, ":")
			continue
		}
		lines[current] = append(lines[current], line)
	}

	sections := make(map[string]string, len(lines))
	for category, ls := range lines {
		if chunk := strings.TrimRight(strings.Join(ls, "\n"), "\n"); chunk != "" {
			sections[category] = chunk
		}
	}
	return sections
}

// isSectionMarker reports whether line is a bare top-level section header,
// e.g. "tasks:" with no indent and nothing else on the line.
func isSectionMarker(line string) bool {
	for _, category := range sectionCategories {
		if line == category+":" {
			return true
		}
	}
	return false
}

// Build renders the full Taskfile.yml body described by cfg.
func (g *Generator) Build(cfg config.File) (string, error) {
	headers, err := g.renderAll("headers", cfg.Headers)
	if err != nil {
		return "", err
	}

	sections := map[string][]string{}
	if err := g.collectSections("includes", cfg.Includes, sections); err != nil {
		return "", err
	}
	if err := g.collectSections("vars", cfg.Vars, sections); err != nil {
		return "", err
	}
	if err := g.collectSections("tasks", cfg.Tasks, sections); err != nil {
		return "", err
	}

	out := []string{strings.Join(headers, "\n\n")}

	if blocks := sections["includes"]; len(blocks) > 0 {
		sorted := sortEntries(strings.Join(blocks, "\n\n"), "\n", nil)
		out = append(out, "includes:\n"+sorted)
	}

	if blocks := sections["vars"]; len(blocks) > 0 {
		sorted := sortEntries(strings.Join(blocks, "\n\n"), "\n", nil)
		out = append(out, "vars:\n"+sorted)
	}

	if blocks := sections["tasks"]; len(blocks) > 0 {
		sorted := sortEntries(strings.Join(blocks, "\n\n"), "\n\n", []string{"default"})
		out = append(out, "tasks:\n"+sorted)
	}

	return strings.Join(out, "\n\n") + "\n", nil
}

// sortEntries splits text into top-level entry blocks (each starting at a
// line matched by entryPattern), sorts them alphabetically by key, and joins
// them back with separator. Any names listed in pinnedFirst are moved to the
// front, in the order given, ahead of the alphabetical ordering.
func sortEntries(text, separator string, pinnedFirst []string) string {
	var blocks [][]string
	for _, line := range strings.Split(text, "\n") {
		switch {
		case entryPattern.MatchString(line):
			blocks = append(blocks, []string{line})
		case len(blocks) > 0:
			blocks[len(blocks)-1] = append(blocks[len(blocks)-1], line)
		}
	}

	sort.SliceStable(blocks, func(i, j int) bool {
		return entryKey(blocks[i]) < entryKey(blocks[j])
	})

	if len(pinnedFirst) > 0 {
		pinnedRank := make(map[string]int, len(pinnedFirst))
		for i, name := range pinnedFirst {
			pinnedRank[strings.ToLower(name)] = i
		}
		sort.SliceStable(blocks, func(i, j int) bool {
			ri, pinnedI := pinnedRank[entryKey(blocks[i])]
			rj, pinnedJ := pinnedRank[entryKey(blocks[j])]
			if pinnedI && pinnedJ {
				return ri < rj
			}
			return pinnedI && !pinnedJ
		})
	}

	formatted := make([]string, 0, len(blocks))
	for _, block := range blocks {
		end := len(block)
		for end > 0 && block[end-1] == "" {
			end--
		}
		formatted = append(formatted, strings.Join(block[:end], "\n"))
	}

	return strings.Join(formatted, separator)
}

// entryKey extracts the lowercase key from an entry block's first line, e.g.
// "  Build:" -> "build", and "  Build: ./build" -> "build".
func entryKey(block []string) string {
	line := strings.TrimSpace(block[0])
	if idx := strings.Index(line, ":"); idx != -1 {
		line = line[:idx]
	}
	return strings.ToLower(line)
}

// ValidateYAML reports an error if content is not syntactically valid YAML.
func ValidateYAML(content string) error {
	var out any
	return yaml.Unmarshal([]byte(content), &out)
}

// Result is the outcome of generating a single named Taskfile.
type Result struct {
	Name    string
	Path    string
	Warning string // non-empty if another file in the same run also writes Path
	Error   error
}

// GenerateAll renders every file described by cfg, under rootDir, validating
// each as YAML. When dryRun is true, files are built and validated but not
// written to disk. If two or more files in cfg share the same Path, every
// one after the first (in name-sorted order) gets a non-empty Warning, since
// generating them in the same run means one silently overwrites another.
// It returns one Result per file (in a stable, name-sorted order) and a
// non-nil error if any file failed.
func GenerateAll(cfg *config.Config, templatesDir, rootDir string, dryRun bool) ([]Result, error) {
	g := New(templatesDir)

	names := make([]string, 0, len(cfg.Files))
	for name := range cfg.Files {
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]Result, 0, len(names))
	seenPaths := make(map[string]string, len(names))
	var failed bool

	for _, name := range names {
		fileCfg := cfg.Files[name]
		result := Result{Name: name, Path: fileCfg.Path}

		if firstName, dup := seenPaths[fileCfg.Path]; dup {
			result.Warning = fmt.Sprintf("output path %q is also written by %q in this run; one will overwrite the other", fileCfg.Path, firstName)
		} else {
			seenPaths[fileCfg.Path] = name
		}

		content, err := g.Build(fileCfg)
		if err == nil && dryRun {
			if verr := ValidateYAML(content); verr != nil {
				err = fmt.Errorf("generated invalid YAML: %w", verr)
			}
		} else if err == nil {
			err = validateAndWrite(content, filepath.Join(rootDir, fileCfg.Path))
		}
		if err != nil {
			result.Error = err
			failed = true
		}

		results = append(results, result)
	}

	if failed {
		return results, fmt.Errorf("one or more Taskfiles failed to generate")
	}
	return results, nil
}

func validateAndWrite(content, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", outPath, err)
	}
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	if err := ValidateYAML(content); err != nil {
		return fmt.Errorf("generated invalid YAML: %w", err)
	}
	return nil
}
