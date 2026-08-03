// Package config parses the gotasky config file, which lists which template
// snippets to combine for each generated Taskfile.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// File describes a single generated Taskfile: where to write it, and which
// template snippets (by filename, under the headers/includes/vars/tasks
// categories) to combine to build it. Sets names entries under the
// top-level Config.Sets to merge in first, ahead of the categories listed
// directly on the file (see resolveSets).
type File struct {
	Path     string   `yaml:"path"`
	Sets     []string `yaml:"sets"`
	Headers  []string `yaml:"headers"`
	Includes []string `yaml:"includes"`
	Vars     []string `yaml:"vars"`
	Tasks    []string `yaml:"tasks"`
}

// Set is a named, reusable bundle of template snippets (by category) that a
// File can pull in via its own Sets field, so common combinations don't
// need to be repeated across every file that uses them.
type Set struct {
	Headers  []string `yaml:"headers"`
	Includes []string `yaml:"includes"`
	Vars     []string `yaml:"vars"`
	Tasks    []string `yaml:"tasks"`
}

// Config is the top-level gotasky config: a named set of File definitions,
// plus gotasky's own optional tool settings.
type Config struct {
	// Templates is the list of template directories to search, in order --
	// overriding the built-in "templates" default. Multiple directories let
	// snippets be spread across more than one location, e.g. a
	// project-specific directory layered on top of a shared one.
	Templates []string        `yaml:"templates"`
	Sets      map[string]Set  `yaml:"sets"`
	Files     map[string]File `yaml:"files"`
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

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := resolveSets(&cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}

// resolveSets expands each file's Sets references into its per-category
// snippet lists: for every set name in File.Sets, in order, that set's
// snippets are prepended ahead of the file's own directly-listed snippets
// (so a file can layer its own additions on top of shared boilerplate).
// Referencing more than one set is allowed; an unknown set name is an
// error, since a config file cannot generate what it can't find.
func resolveSets(cfg *Config) error {
	for name, f := range cfg.Files {
		if len(f.Sets) == 0 {
			continue
		}

		var headers, includes, vars, tasks []string
		for _, setName := range f.Sets {
			set, ok := cfg.Sets[setName]
			if !ok {
				return fmt.Errorf("file %q references unknown set %q", name, setName)
			}
			headers = append(headers, set.Headers...)
			includes = append(includes, set.Includes...)
			vars = append(vars, set.Vars...)
			tasks = append(tasks, set.Tasks...)
		}

		f.Headers = append(headers, f.Headers...)
		f.Includes = append(includes, f.Includes...)
		f.Vars = append(vars, f.Vars...)
		f.Tasks = append(tasks, f.Tasks...)
		cfg.Files[name] = f
	}
	return nil
}
