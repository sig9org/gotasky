// Package generator combines Taskfile Root Schema template snippets into a
// complete Taskfile.yml, sorting map entries and
// validating the result as YAML.
package generator

import (
	"bytes"
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

// sectionCategories are map-valued top-level Taskfile keys a template snippet may
// declare for itself, in addition to the category folder it's read from —
// e.g. a task definition under templates/tasks/ that also needs its own
// "vars:" entry.
var sectionCategories = []string{"includes", "vars", "env", "tasks"}

// rootCategories defines both the generated Taskfile property order and the
// directory names scanned below each configured templates directory.
var rootCategories = []string{
	"version", "dotenv", "env", "includes", "interval", "method", "output",
	"run", "set", "shopt", "silent", "vars", "tasks",
}

// templateCategories contains every supported Taskfile Root Schema directory.
var templateCategories = rootCategories

// Generator renders Taskfile bodies from template snippets under
// TemplatesDirs, a search path of one or more directories: for a given
// snippet, each directory is tried in order and the first match wins. This
// lets snippets be spread across more than one location, e.g. a
// project-specific directory layered on top of a shared one.
type Generator struct {
	TemplatesDirs []string
}

// UnusedTemplate is a template file found under one of the configured
// templates directories that was not selected by any generated file.
type UnusedTemplate struct {
	Dir      string
	Category string
	Name     string
}

// New creates a Generator that reads template snippets from templatesDirs,
// in search-path order.
func New(templatesDirs []string) *Generator {
	return &Generator{TemplatesDirs: templatesDirs}
}

// FindUnusedTemplates returns template files that are not used by any file in
// cfg. A requested snippet is attributed to the first configured directory
// where it exists, matching readTemplate's search-path behavior. Missing
// directories and category folders are ignored. Files listed in cfg.Ignore
// are excluded from the report.
func FindUnusedTemplates(cfg *config.Config, templatesDirs []string) []UnusedTemplate {
	ignored := make(map[string]bool, len(cfg.Ignore))
	for _, name := range cfg.Ignore {
		ignored[name] = true
	}

	used := make(map[string]bool)
	for _, fileCfg := range cfg.Files {
		for _, category := range templateCategories {
			for _, name := range fileCfg.TemplateNames(category) {
				for _, dir := range templatesDirs {
					path := filepath.Join(dir, category, name)
					if info, err := os.Stat(path); err == nil && !info.IsDir() {
						used[path] = true
						break
					}
				}
			}
		}
	}

	var unused []UnusedTemplate
	for _, dir := range templatesDirs {
		for _, category := range templateCategories {
			entries, err := os.ReadDir(filepath.Join(dir, category))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || ignored[entry.Name()] {
					continue
				}
				path := filepath.Join(dir, category, entry.Name())
				if !used[path] {
					unused = append(unused, UnusedTemplate{Dir: dir, Category: category, Name: entry.Name()})
				}
			}
		}
	}

	sort.Slice(unused, func(i, j int) bool {
		if unused[i].Dir != unused[j].Dir {
			return unused[i].Dir < unused[j].Dir
		}
		if unused[i].Category != unused[j].Category {
			return unused[i].Category < unused[j].Category
		}
		return unused[i].Name < unused[j].Name
	})
	return unused
}

func (g *Generator) readTemplate(category, name string) (string, error) {
	tried := make([]string, 0, len(g.TemplatesDirs))
	for _, dir := range g.TemplatesDirs {
		path := filepath.Join(dir, category, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimRight(string(data), "\n"), nil
		}
		tried = append(tried, path)
	}
	return "", fmt.Errorf("read template %s/%s: not found in %s", category, name, strings.Join(tried, ", "))
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
// results into dest, keyed by section name ("includes", "vars", "env", or "tasks").
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
// that is exactly "includes:", "vars:", "env:", or "tasks:" at column 0 switches the
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
	sections := map[string][]string{}
	for _, category := range sectionCategories {
		if err := g.collectSections(category, cfg.TemplateNames(category), sections); err != nil {
			return "", err
		}
	}

	var out []string
	for _, category := range rootCategories {
		if isSectionCategory(category) {
			blocks := sections[category]
			if len(blocks) == 0 {
				continue
			}
			separator := "\n"
			var pinned []string
			if category == "tasks" {
				separator = "\n\n"
				pinned = []string{"default"}
			}
			out = append(out, category+":\n"+sortEntries(strings.Join(blocks, "\n\n"), separator, pinned))
			continue
		}

		// Scalar/list/object root-property snippets are complete YAML fragments.
		// Merge them into one top-level property so multiple snippets can never
		// emit duplicate root keys.
		blocks, err := g.renderAll(category, cfg.TemplateNames(category))
		if err != nil {
			return "", err
		}
		block, err := mergeRootFragments(category, blocks)
		if err != nil {
			return "", err
		}
		if block != "" {
			out = append(out, block)
		}
	}

	return strings.Join(out, "\n\n") + "\n", nil
}

// mergeRootFragments combines complete YAML fragments for one root property.
// Sequences are concatenated, mappings are recursively merged, and scalar
// values use the last configured value (allowing a referencing preset or file
// to override an inherited default).
func mergeRootFragments(category string, blocks []string) (string, error) {
	var merged *yaml.Node
	var keyNode *yaml.Node
	for _, block := range blocks {
		var document yaml.Node
		if err := yaml.Unmarshal([]byte(block), &document); err != nil {
			return "", fmt.Errorf("parse template for %s: %w", category, err)
		}
		if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
			return "", fmt.Errorf("template for %s must contain a top-level %q property", category, category)
		}
		mapping := document.Content[0]
		var value *yaml.Node
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == category {
				keyNode = cloneYAMLNode(mapping.Content[i])
				value = mapping.Content[i+1]
				break
			}
		}
		if value == nil {
			return "", fmt.Errorf("template for %s does not define top-level property %q", category, category)
		}
		merged = mergeYAMLNodes(merged, value)
	}
	if merged == nil {
		return "", nil
	}

	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			keyNode,
			merged,
		},
	}}}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return "", fmt.Errorf("render merged %s property: %w", category, err)
	}
	return strings.TrimRight(output.String(), "\n"), nil
}

func mergeYAMLNodes(current, incoming *yaml.Node) *yaml.Node {
	if current == nil || current.Kind != incoming.Kind {
		return cloneYAMLNode(incoming)
	}
	switch incoming.Kind {
	case yaml.SequenceNode:
		for _, item := range incoming.Content {
			current.Content = append(current.Content, cloneYAMLNode(item))
		}
		return current
	case yaml.MappingNode:
		for i := 0; i+1 < len(incoming.Content); i += 2 {
			incomingKey, incomingValue := incoming.Content[i], incoming.Content[i+1]
			found := false
			for j := 0; j+1 < len(current.Content); j += 2 {
				if current.Content[j].Value == incomingKey.Value {
					current.Content[j+1] = mergeYAMLNodes(current.Content[j+1], incomingValue)
					found = true
					break
				}
			}
			if !found {
				current.Content = append(current.Content, cloneYAMLNode(incomingKey), cloneYAMLNode(incomingValue))
			}
		}
		return current
	default:
		return cloneYAMLNode(incoming)
	}
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = cloneYAMLNode(child)
	}
	return &clone
}

func isSectionCategory(category string) bool {
	for _, candidate := range sectionCategories {
		if category == candidate {
			return true
		}
	}
	return false
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

// checkNoDuplicateTemplates reports an error if any template category
// folder (all Root Schema properties) has the same filename present in
// more than one of templatesDirs. TemplatesDirs is meant to be a search
// path for filling in snippets *missing* from one directory (e.g. a
// project-specific directory layered on a shared one), not a place to
// silently shadow one directory's snippet with another's — so a filename
// duplicated across directories is a configuration mistake, reported
// up front rather than resolved by "first directory wins". A listed
// directory that doesn't exist, or has no folder for a given category, is
// skipped rather than treated as an error. Filenames listed in ignore
// (config.Config.Ignore, e.g. stray ".DS_Store"/".gitkeep" files) are
// skipped entirely, as if they weren't there.
func checkNoDuplicateTemplates(templatesDirs, ignore []string) error {
	ignored := make(map[string]bool, len(ignore))
	for _, name := range ignore {
		ignored[name] = true
	}

	type problem struct {
		label string // "<category>/<filename>"
		dirs  string
	}
	var problems []problem

	for _, category := range templateCategories {
		locations := map[string][]string{}
		for _, dir := range templatesDirs {
			entries, err := os.ReadDir(filepath.Join(dir, category))
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() || ignored[entry.Name()] {
					continue
				}
				locations[entry.Name()] = append(locations[entry.Name()], dir)
			}
		}

		names := make([]string, 0, len(locations))
		for name := range locations {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			dirs := locations[name]
			if len(dirs) > 1 {
				problems = append(problems, problem{
					label: category + "/" + name,
					dirs:  strings.Join(dirs, ", "),
				})
			}
		}
	}

	if len(problems) == 0 {
		return nil
	}

	// Right-pad every label to the widest one so the "->" arrows line up
	// into a column, regardless of category/filename length.
	maxLabel := 0
	for _, p := range problems {
		if len(p.label) > maxLabel {
			maxLabel = len(p.label)
		}
	}
	lines := make([]string, len(problems))
	for i, p := range problems {
		lines[i] = fmt.Sprintf("%-*s -> %s", maxLabel, p.label, p.dirs)
	}

	return fmt.Errorf("Duplicate template filenames across template directories:\n  %s", strings.Join(lines, "\n  "))
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
	Warning string // non-empty for non-fatal configuration concerns
	Error   error
}

// GenerateAll renders every file described by cfg, under rootDir, validating
// each as YAML. It first checks templatesDirs for any template category
// (all Root Schema properties) that has the same filename in more than
// one directory, returning a nil result slice and an error immediately if
// so — see checkNoDuplicateTemplates. Filenames listed in cfg.Ignore are
// excluded from that check, as if they didn't exist. When dryRun is true,
// files are built and validated but not written to disk. If two or more
// files in cfg share the same Path, every one after the first (in
// name-sorted order) gets a non-empty Warning, since generating them in the
// same run means one silently overwrites another. It returns one Result
// per file (in a stable, name-sorted order) and a non-nil error if any file
// failed.
func GenerateAll(cfg *config.Config, templatesDirs []string, rootDir string, dryRun bool) ([]Result, error) {
	if err := checkNoDuplicateTemplates(templatesDirs, cfg.Ignore); err != nil {
		return nil, err
	}

	g := New(templatesDirs)

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
		if len(fileCfg.Warnings) > 0 {
			result.Warning = strings.Join(fileCfg.Warnings, "; ")
		}

		if firstName, dup := seenPaths[fileCfg.Path]; dup {
			pathWarning := fmt.Sprintf("output path %q is also written by %q in this run; one will overwrite the other", fileCfg.Path, firstName)
			if result.Warning != "" {
				result.Warning += "; "
			}
			result.Warning += pathWarning
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
