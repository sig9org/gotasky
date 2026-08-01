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

// Config is the top-level gotasky config: a named set of File definitions.
type Config struct {
	Files map[string]File `yaml:"files"`
}

// Load reads and parses a gotasky config file from path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}
