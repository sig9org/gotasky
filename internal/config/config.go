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
// categories) to combine to build it.
type File struct {
	Path     string   `yaml:"path"`
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

	return &cfg, nil
}
