// Package config parses the gotasky config file, which lists which template
// snippets to combine for each generated Taskfile.
package config

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// File describes a single generated Taskfile: where to write it, and which
// template snippets (by filename, under Taskfile Root Schema property
// categories) to combine to build it. Presets names entries under the
// top-level Config.Presets to merge in first, ahead of the categories listed
// directly on the file (see resolvePresets).
type File struct {
	Path     string   `yaml:"path"`
	Presets  []string `yaml:"presets"`
	Version  []string `yaml:"version"`
	Output   []string `yaml:"output"`
	Method   []string `yaml:"method"`
	Includes []string `yaml:"includes"`
	Vars     []string `yaml:"vars"`
	Env      []string `yaml:"env"`
	Tasks    []string `yaml:"tasks"`
	Silent   []string `yaml:"silent"`
	Dotenv   []string `yaml:"dotenv"`
	Run      []string `yaml:"run"`
	Interval []string `yaml:"interval"`
	Set      []string `yaml:"set"`
	Shopt    []string `yaml:"shopt"`
	Warnings []string `yaml:"-"`
}

// Preset is a named, reusable bundle of template snippets (by category) that a
// File can pull in via its own Presets field, so common combinations don't
// need to be repeated across every file that uses them.
type Preset struct {
	Version  []string `yaml:"version"`
	Output   []string `yaml:"output"`
	Method   []string `yaml:"method"`
	Includes []string `yaml:"includes"`
	Vars     []string `yaml:"vars"`
	Env      []string `yaml:"env"`
	Tasks    []string `yaml:"tasks"`
	Silent   []string `yaml:"silent"`
	Dotenv   []string `yaml:"dotenv"`
	Run      []string `yaml:"run"`
	Interval []string `yaml:"interval"`
	Set      []string `yaml:"set"`
	Shopt    []string `yaml:"shopt"`
}

// Config is the top-level gotasky config: a named collection of File definitions,
// plus gotasky's own optional tool settings.
type Config struct {
	// Templates is the list of template directories to search, in order --
	// overriding the built-in "templates" default. Multiple directories let
	// snippets be spread across more than one location, e.g. a
	// project-specific directory layered on top of a shared one.
	Templates []string `yaml:"templates"`
	// Ignore lists filenames (e.g. stray ".DS_Store"/".gitkeep" files sitting
	// in a templates directory) to exclude from every templatesDirs
	// directory scan -- currently just the duplicate-filename check -- so
	// they're never treated as snippets. Omitting it processes every file as
	// before.
	Ignore  []string          `yaml:"ignore"`
	Presets map[string]Preset `yaml:"presets"`
	Files   map[string]File   `yaml:"files"`
}

// Load reads and parses a gotasky config file from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// A top-level "config" key would mean the config file names its own
	// path from inside itself, which is contradictory (by the time it's
	// read, that path has already been resolved): reject it explicitly
	// rather than silently ignoring it, which -- since Config has no such
	// field -- is what plain unmarshaling would otherwise do.
	var probe map[string]any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if _, ok := probe["config"]; ok {
		return nil, fmt.Errorf("parse config %s: a top-level \"config\" key is not supported -- a config file cannot specify its own path; use -c/-config instead", path)
	}
	if _, ok := probe["sets"]; ok {
		return nil, fmt.Errorf("parse config %s: \"sets\" has been renamed to \"presets\"", path)
	}
	if files, ok := probe["files"].(map[string]any); ok {
		for name, rawFile := range files {
			if file, ok := rawFile.(map[string]any); ok {
				if _, exists := file["sets"]; exists {
					return nil, fmt.Errorf("parse config %s: file %q uses \"sets\", which has been renamed to \"presets\"", path, name)
				}
				if _, exists := file["headers"]; exists {
					return nil, fmt.Errorf("parse config %s: file %q uses removed \"headers\"; use the Root Schema property \"version\" instead", path, name)
				}
			}
		}
	}
	if presets, ok := probe["presets"].(map[string]any); ok {
		for name, rawPreset := range presets {
			if preset, ok := rawPreset.(map[string]any); ok {
				if _, exists := preset["headers"]; exists {
					return nil, fmt.Errorf("parse config %s: preset %q uses removed \"headers\"; use the Root Schema property \"version\" instead", path, name)
				}
			}
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := resolvePresets(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}

// resolvePresets expands each file's Presets references into its per-category
// snippet lists: for every preset name in File.Presets, in order, that preset's
// snippets are prepended ahead of the file's own directly-listed snippets
// (so a file can layer its own additions on top of shared boilerplate).
// Referencing more than one preset is allowed; an unknown preset name is an
// error, since a config file cannot generate what it can't find.
func resolvePresets(cfg *Config) error {
	for name, f := range cfg.Files {
		if len(f.Presets) == 0 {
			continue
		}

		var merged Preset
		var mergedNames []string
		for _, presetName := range f.Presets {
			preset, ok := cfg.Presets[presetName]
			if !ok {
				return fmt.Errorf("file %q references unknown preset %q", name, presetName)
			}
			duplicateOf := ""
			for i, previous := range mergedNames {
				if presetName == previous || presetsEqual(preset, cfg.Presets[previous]) {
					duplicateOf = mergedNames[i]
					break
				}
			}
			if duplicateOf != "" {
				f.Warnings = append(f.Warnings, fmt.Sprintf("presets %q and %q contain exactly the same elements; %q was skipped", duplicateOf, presetName, presetName))
				continue
			}
			mergedNames = append(mergedNames, presetName)
			appendPreset(&merged, preset)
		}

		prependPreset(&f, merged)
		cfg.Files[name] = f
	}
	return nil
}

func presetsEqual(a, b Preset) bool {
	return slices.Equal(a.Version, b.Version) && slices.Equal(a.Output, b.Output) &&
		slices.Equal(a.Method, b.Method) &&
		slices.Equal(a.Includes, b.Includes) && slices.Equal(a.Vars, b.Vars) &&
		slices.Equal(a.Env, b.Env) && slices.Equal(a.Tasks, b.Tasks) &&
		slices.Equal(a.Silent, b.Silent) && slices.Equal(a.Dotenv, b.Dotenv) &&
		slices.Equal(a.Run, b.Run) && slices.Equal(a.Interval, b.Interval) &&
		slices.Equal(a.Set, b.Set) && slices.Equal(a.Shopt, b.Shopt)
}

func appendPreset(dst *Preset, src Preset) {
	dst.Version = append(dst.Version, src.Version...)
	dst.Output = append(dst.Output, src.Output...)
	dst.Method = append(dst.Method, src.Method...)
	dst.Includes = append(dst.Includes, src.Includes...)
	dst.Vars = append(dst.Vars, src.Vars...)
	dst.Env = append(dst.Env, src.Env...)
	dst.Tasks = append(dst.Tasks, src.Tasks...)
	dst.Silent = append(dst.Silent, src.Silent...)
	dst.Dotenv = append(dst.Dotenv, src.Dotenv...)
	dst.Run = append(dst.Run, src.Run...)
	dst.Interval = append(dst.Interval, src.Interval...)
	dst.Set = append(dst.Set, src.Set...)
	dst.Shopt = append(dst.Shopt, src.Shopt...)
}

func prependPreset(f *File, preset Preset) {
	f.Version = append(preset.Version, f.Version...)
	f.Output = append(preset.Output, f.Output...)
	f.Method = append(preset.Method, f.Method...)
	f.Includes = append(preset.Includes, f.Includes...)
	f.Vars = append(preset.Vars, f.Vars...)
	f.Env = append(preset.Env, f.Env...)
	f.Tasks = append(preset.Tasks, f.Tasks...)
	f.Silent = append(preset.Silent, f.Silent...)
	f.Dotenv = append(preset.Dotenv, f.Dotenv...)
	f.Run = append(preset.Run, f.Run...)
	f.Interval = append(preset.Interval, f.Interval...)
	f.Set = append(preset.Set, f.Set...)
	f.Shopt = append(preset.Shopt, f.Shopt...)
}

// TemplateNames returns the snippet filenames configured for a Taskfile root
// schema property.
func (f File) TemplateNames(category string) []string {
	switch strings.ToLower(category) {
	case "version":
		return f.Version
	case "output":
		return f.Output
	case "method":
		return f.Method
	case "includes":
		return f.Includes
	case "vars":
		return f.Vars
	case "env":
		return f.Env
	case "tasks":
		return f.Tasks
	case "silent":
		return f.Silent
	case "dotenv":
		return f.Dotenv
	case "run":
		return f.Run
	case "interval":
		return f.Interval
	case "set":
		return f.Set
	case "shopt":
		return f.Shopt
	default:
		return nil
	}
}
