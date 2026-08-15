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

1. You keep template snippets under a `templates/` directory, grouped by every
   [Taskfile Root Schema](https://taskfile.dev/docs/reference/schema#root-schema)
   property: `version`, `output`, `method`, `includes`, `vars`, `env`, `tasks`,
   `silent`, `dotenv`, `run`, `interval`, `set`, and `shopt`. Missing category
   directories are skipped.
2. You write a config file (`config.yml` by default) listing one or more output files, each naming which snippets to combine:

   The supported category keys are the 13 Root Schema properties above plus
   gotasky's `presets`, for 14 keys in total.

   ```yaml
   presets:
     common:
       vars:
         - cleanup.txt
       tasks:
         - default.txt
         - time.txt
         - cleanup.txt

   files:
     os-update:
       path: "taskfiles/os-update/Taskfile.yml"
       presets: [common]
       version:
         - default.txt
       includes:
         - common.txt
       vars:
         - os-update.txt
       tasks:
         - os-update.txt

     terraform:
       path: "taskfiles/terraform/Taskfile.yml"
       presets: [common]
       version:
         - env.txt
       tasks:
         - terraform.txt
   ```

3. Running `gotasky` reads the config, concatenates the referenced snippets for each entry, and writes the result to `path`.

See [`config.yml.example`](config.yml.example) for a working example using this repo's own template snippets.

For every generated file, gotasky guarantees:

- Every Root Schema property appears at most once at the top level. Sequence
  properties (`dotenv`, `set`, `shopt`) are concatenated, mapping properties
  are merged, and later scalar values override earlier inherited defaults.
- **`includes:`** entries are sorted alphabetically (case-insensitive).
- **`vars:`** entries are sorted alphabetically (case-insensitive).
- **`tasks:`** entries are sorted alphabetically, except the `default` task, which is always placed first.
- The generated content is parsed as YAML before being reported as OK; a snippet that produces invalid YAML is reported as a failure (`[NG]`), with the parse error printed to stderr, instead of silently corrupting the output.
- A snippet may declare its own `vars:`, `tasks:`, and/or `includes:` line(s) — e.g. a snippet under `templates/tasks/` that needs a variable only that task uses can include its own `vars:` block alongside its `tasks:` block. Each labeled part is merged into the matching top-level section instead of duplicating the key or corrupting the section it was combined into.
- If two or more files in the config share the same `path`, every one after the first (alphabetically, by file name) prints an orange `[WARN]` to stderr, since one silently overwrites the other's output in the same run. This is a warning, not a failure — it doesn't affect the exit code.
- If the same filename shows up in more than one [configured templates directory](#configuring-paths), for the same category, that's a configuration error — see below — and fails the whole run before any file is generated.

`gotasky` exits `0` only if every file in the config generated successfully; if any file fails, it exits non-zero (and still reports every file's individual `[OK]`/`[NG]` status).

### Output and colors

Every status/diagnostic line gotasky prints is colored consistently, whether or not stdout/stderr is a terminal:

| Prefix    | Color   | Meaning                                                                 |
| --------- | ------- | ------------------------------------------------------------------------ |
| `[OK]`    | none    | a file generated successfully                                          |
| `[WARN]`  | orange  | non-fatal issue, e.g. two files writing the same output `path`         |
| `[NG]`    | red     | a single file failed to generate (e.g. a missing snippet, invalid YAML)|
| `[ERROR]` | red     | the whole run failed before any file was generated (e.g. an invalid config file, or [duplicate template filenames](#configuring-paths)) |
| `[DEBUG]` | gray    | `-debug` diagnostics, each timestamped                                 |

### Reusing snippet lists with presets

If several output files share the same handful of snippets (as `os-update` and `terraform` do above), you can factor them out once as a named **preset**, via an optional top-level `presets` key, instead of repeating them in every file:

```yaml
presets:
  common:
    version: [default.txt]   # any supported Root Schema category
    vars: [cleanup.txt]
    tasks: [default.txt, time.txt, cleanup.txt]
```

Any file can then pull a preset in via its own `presets` key:

```yaml
files:
  os-update:
    path: "taskfiles/os-update/Taskfile.yml"
    presets: [common]
    tasks: [os-update.txt]
```

A preset can also reference one or more other presets. Referenced presets are
expanded first, in the listed order, followed by the referencing preset's own
snippets:

```yaml
presets:
  default:
    version: [default.yml]
    dotenv: [dotenv.yml]
  project:
    presets: [default]
    dotenv: [project.yml]
```

Unknown preset references and circular references are configuration errors.

A file can reference more than one preset (`presets: [common, extra]`); each preset's snippets are merged in, in the order listed, ahead of the file's own directly-listed snippets, per category. Referencing a preset name that isn't defined under `presets` is a config error. If the same preset is referenced more than once, or two referenced presets contain exactly the same elements, gotasky prints `[WARN]`, merges that content once, and continues successfully.

The former `sets` key has been renamed to `presets` to distinguish it from the Taskfile Root Schema's singular `set` property. Using the old key returns an error with migration guidance instead of silently ignoring it.

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

`-silent` suppresses the `[OK]` lines gotasky normally prints to stdout; `[WARN]`/`[NG]`/`[ERROR]` failures are always reported on stderr regardless, and the exit code is unaffected. If `-debug` is also given, `-debug` wins: silent is ignored and normal (`[DEBUG]` and `[OK]`) output is printed.

For each file, `-debug` prints all 14 supported elements (`presets` plus the 13 Root Schema categories), including empty lists. When presets are referenced, it also prints all 14 elements for each resolved preset — not just the file's final, already-merged list.

### Configuring paths

By default, gotasky looks for:

- its config file at `./config.yml`, falling back to `./config.yaml` if `config.yml` doesn't exist
- its template directories at `./templates`

The config file path can be set with `-c`/`-config`; there's no other way to point at one (no settings file, no environment variable) — the flag and the two built-in filenames are the whole story. A config file cannot name its own path from inside itself, either: a top-level `config` key is rejected with an error rather than silently ignored, since that would be contradictory (the path has to be resolved before the file can even be read).

The template directories are configured from *inside* the config file itself, via an optional top-level `templates` key — a YAML list of one or more directories, searched in order. A directory later in the list fills in a snippet that's *missing* from an earlier one, so a project-specific directory can be layered on top of a shared one; a listed directory that doesn't exist is simply skipped, not an error — generation only fails if a snippet isn't found in any of them:

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

This search path is only for filling gaps, never for shadowing: if the same filename exists in more than one configured directory for any supported Root Schema category, that's a configuration mistake rather than a "first directory wins" case. Generation fails up front, before any file is built, with an `[ERROR]` naming every duplicate and the directories it was found in:

```
[ERROR]Duplicate template filenames across template directories:
  version/default.yml -> templates/shared, templates/project
  tasks/cleanup.yml   -> templates/shared, templates/project
```

#### Ignoring stray files

An optional top-level `ignore` key lists filenames to exclude from every configured templates directory — useful for stray files an editor or OS drops into them (`.DS_Store`, `.gitkeep`, ...) that aren't real snippets. Ignored filenames are skipped by the duplicate-filename check above, as if they weren't there:

```yaml
# config.yml
templates:
  - templates/shared
  - templates/project
ignore:
  - .DS_Store
  - .gitkeep
```

If `ignore` is omitted, every file is processed as before — nothing is excluded.

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
