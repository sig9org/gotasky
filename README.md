<p align="center">
  <img src="https://raw.githubusercontent.com/sig9org/gotasky/main/assets/logo.webp" alt="gotasky">
</p>

# gotasky

[![Go Reference](https://pkg.go.dev/badge/github.com/sig9org/gotasky.svg)](https://pkg.go.dev/github.com/sig9org/gotasky)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A modular `Taskfile.yml` generator for [go-task](https://taskfile.dev). Combine reusable header, variable, and task snippets into one or more complete Taskfiles, instead of copy-pasting the same boilerplate (timing helpers, cleanup tasks, OS-update tasks, ...) across every project.

## Why

If you maintain several projects with `go-task`, you tend to reuse the same handful of tasks (cleanup, OS package updates, Terraform wrappers, timing wrappers, ...) with only minor differences. gotasky lets you keep each of those as a small snippet under `templates/`, then declare per-project which snippets to combine and where to write the result. Update a snippet once, regenerate, and every Taskfile that uses it picks up the change.

## How it works

1. You keep template snippets under a `templates/` directory, grouped into four categories:
   - `templates/headers/*.txt` — the top of the file (`version`, `silent`, `dotenv`, ...)
   - `templates/includes/*.txt` — entries for the `includes:` section
   - `templates/vars/*.txt` — entries for the `vars:` section
   - `templates/tasks/*.txt` — entries for the `tasks:` section
2. You write a config file (`config.yml` by default) listing one or more output files, each naming which snippets to combine:

   ```yaml
   files:
     os-update:
       path: "taskfiles/os-update/Taskfile.yml"
       headers:
         - default.txt
       includes:
         - common.txt
       vars:
         - cleanup.txt
         - os-update.txt
       tasks:
         - default.txt
         - time.txt
         - cleanup.txt
         - os-update.txt

     terraform:
       path: "taskfiles/terraform/Taskfile.yml"
       headers:
         - env.txt
       vars:
         - cleanup.txt
       tasks:
         - default.txt
         - time.txt
         - cleanup.txt
         - terraform.txt
   ```

3. Running `gotasky` reads the config, concatenates the referenced snippets for each entry, and writes the result to `path`.

See [`config.yml.example`](config.yml.example) for a working example using this repo's own template snippets.

For every generated file, gotasky guarantees:

- **`includes:`** entries are sorted alphabetically (case-insensitive).
- **`vars:`** entries are sorted alphabetically (case-insensitive).
- **`tasks:`** entries are sorted alphabetically, except the `default` task, which is always placed first.
- The generated content is parsed as YAML before being reported as OK; a snippet that produces invalid YAML is reported as a failure (`[NG]`), with the parse error printed to stderr, instead of silently corrupting the output.
- A snippet may declare its own `vars:`, `tasks:`, and/or `includes:` line(s) — e.g. a snippet under `templates/tasks/` that needs a variable only that task uses can include its own `vars:` block alongside its `tasks:` block. Each labeled part is merged into the matching top-level section instead of duplicating the key or corrupting the section it was combined into.
- If two or more files in the config share the same `path`, every one after the first (alphabetically, by file name) prints an orange `[WARN]` to stderr, since one silently overwrites the other's output in the same run. This is a warning, not a failure — it doesn't affect the exit code.

`gotasky` exits `0` only if every file in the config generated successfully; if any file fails, it exits non-zero (and still reports every file's individual `[OK]`/`[NG]` status).

## Sample generated Taskfiles

Example `Taskfile.yml` output produced by gotasky is published at [sig9org/taskfiles](https://github.com/sig9org/taskfiles).

## Installation

Download a prebuilt binary from the [releases page](https://github.com/sig9org/gotasky/releases), or build from source:

```sh
git clone https://github.com/sig9org/gotasky.git
cd gotasky
go build -o gotasky .
```

## Usage

```sh
gotasky                    # generate Taskfiles from ./config.yml (or ./config.yaml) using ./templates
gotasky -c my-config.yml   # generate using a specific config file
gotasky -dryrun            # dry run: validate generation without writing any files
gotasky -debug             # print detailed debug information while generating
gotasky -silent            # suppress standard output
gotasky -update            # update gotasky itself to the latest GitHub release
gotasky -v                 # print the gotasky version
gotasky -h                 # show help
```

```
% gotasky --help
gotasky v0.0.1 (a1b2c3d4e5f6...)

Usage of gotasky:
  -c, -config <path>  path to the config file
      -dryrun         validate generation without writing any files
      -debug          print detailed debug information
      -silent         suppress standard output (overridden by -debug)
      -update         update gotasky itself to the latest GitHub release
  -v, -version        print the gotasky version
  -h, -help           show this help message
```

`-silent` suppresses the `[OK]` lines gotasky normally prints to stdout; `[NG]` failures are always reported on stderr regardless, and the exit code is unaffected. If `-debug` is also given, `-debug` wins: silent is ignored and normal (`[DEBUG]` and `[OK]`) output is printed.

### Configuring paths

By default, gotasky looks for:

- its config file at `./config.yml`, falling back to `./config.yaml` if `config.yml` doesn't exist
- its template directories at `./templates`

The config file path can be set with `-c`/`-config`; there's no other way to point at one (no settings file, no environment variable) — the flag and the two built-in filenames are the whole story. A config file cannot name its own path from inside itself, either: a top-level `config` key is rejected with an error rather than silently ignored, since that would be contradictory (the path has to be resolved before the file can even be read).

The template directories are configured from *inside* the config file itself, via an optional top-level `templates` key — a YAML list of one or more directories, searched in order. The first directory that has a given snippet wins, so a project-specific directory can be layered on top of a shared one; a listed directory that doesn't exist is simply skipped, not an error — generation only fails if a snippet isn't found in any of them:

```yaml
# config.yml
templates:
  - templates
  # - project-templates
  # - shared-templates

files:
  os-update:
    path: "taskfiles/os-update/Taskfile.yml"
    # ...
```

If `templates` is omitted, gotasky falls back to `./templates`.

### Self-update

`gotasky -update` checks the [sig9org/gotasky](https://github.com/sig9org/gotasky) releases page for a newer version matching your OS/arch and, if found, replaces the running binary in place. Builds without a real release version (e.g. built locally with `go build`, where the version defaults to `dev`) skip self-update instead of erroring.

## Development

This repository's own `Taskfile.yml` is used to build and test gotasky itself:

```sh
task                    # list all available tasks
task go-build (gb)      # build ./dist/gotasky for the current platform
task go-all-build (ga)  # cross-compile release binaries for all platforms into ./dist
task go-test (gt)       # go vet ./... && go test ./...
task go-clean (gc)      # empty ./dist
task go-register (gr)   # register the latest tagged version on pkg.go.dev
task go-version (gv)    # print version info (builds and runs with -v)
task cleanup (c)        # remove editor/OS cruft (.DS_Store, .terraform, ...)
```

## License

MIT — see [LICENSE](LICENSE).
