# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gotasky` is a Go CLI that generates one or more `Taskfile.yml` files (for [go-task](https://taskfile.dev)) by combining reusable snippet files. Instead of copy-pasting the same boilerplate (cleanup tasks, timing wrappers, OS-update tasks, ...) across many projects, you keep each snippet once under `templates/` and declare per-output-file which snippets to combine in a YAML config.

## Commands

This repo uses [go-task](https://taskfile.dev) (`Taskfile.yml`) for its own build/test, not raw `go` commands directly, though both work.

```sh
task build       # build ./dist/gotasky for the current platform, with -ldflags version injection
task build-all   # cross-compile release binaries for all platforms into ./dist
task test        # go vet ./... && go test ./...
task cleanup     # remove editor/OS cruft (.DS_Store, .terraform, ...)
task             # list all available tasks (default)
```

Equivalent raw Go commands:

```sh
go vet ./...
go test ./...
go test ./... -run TestName          # single test across all packages
go test ./internal/generator/...     # single package
go build -o gotasky .
```

`BINARY_NAME` and `VERSION_PKG` used by the `build`/`build-all` tasks are defined in `project.ini` (loaded via `dotenv` in `Taskfile.yml`).

## Architecture

Entry point `gotasky.go` just loads `.env` (via `godotenv`) and delegates everything to `internal/cli.Run(args, stdout, stderr) int`. All four packages under `internal/` have a single, narrow responsibility:

- **`internal/config`** — `Load(path)` parses the YAML config file into `Config{Files map[string]File}`. Each `File` names an output `Path` and, per category (`Headers`, `Includes`, `Vars`, `Tasks`), which snippet filenames to combine.
- **`internal/generator`** — the core logic. `Generator.Build(cfg config.File)` reads snippet files from `TemplatesDir/<category>/<name>` and assembles the final Taskfile body; `GenerateAll(cfg, templatesDir, rootDir, dryRun)` does this for every file in the config, sorted by name, writing (or, in dry-run mode, just validating) each one.
- **`internal/selfupdate`** — wraps `go-selfupdate` to check GitHub releases and replace the running binary; treats a non-semver `Version` (i.e. `"dev"`, the default for local builds) as "not a release build" rather than erroring against the network.
- **`internal/cli`** — flag parsing and wiring; `Run` is the whole CLI surface, tested by driving it end-to-end (`Run([]string{...}, &stdout, &stderr)`) rather than unit-testing flag parsing in isolation.
- **`internal/version`** — a single `Version` var overwritten at build time via `-ldflags "-X .../internal/version.Version=..."`.

### Template categories and merging (the part worth understanding before touching `generator.go`)

Templates live under `templates/<category>/*` in four categories: `headers`, `includes`, `vars`, `tasks`. For a given output file, `Build` renders `headers` snippets as-is (concatenated, verbatim — this becomes the top of the file: `version`, `silent`, `dotenv`, ...), and for `includes`/`vars`/`tasks` it:

1. Reads every named snippet for that category.
2. Runs each through `splitSections`, which reassigns lines to a different section whenever it hits a bare `vars:`, `tasks:`, or `includes:` marker at column 0 mid-file — so a snippet under `templates/tasks/` can also declare its own `vars:` block (e.g. a variable only that task needs), and it gets merged into the top-level `vars:` section rather than duplicating a `tasks:` key or corrupting the tasks section. Content before the first such marker (or all of it, if there's none) is attributed to the snippet's own category folder.
3. Pools the resulting chunks per section across *all* snippets/categories, then sorts entries alphabetically (case-insensitive) via `sortEntries`/`entryPattern` — `tasks:` pins `default` first, `includes:`/`vars:` are plain alphabetical.
4. Wraps each non-empty pooled section in its single top-level key (`includes:`, `vars:`, `tasks:`) and joins everything, then YAML-validates the result before writing.

This section-splitting logic is why a naive re-implementation ("just glob and concatenate the files") would silently break: two snippets that both start with `tasks:` (common if they were copy-pasted from a full Taskfile) must not produce two `tasks:` keys, and a `vars:` block embedded inside a `tasks/*.yml` snippet must land in the `vars:` section, not get parsed as a bogus task entry.

`entryPattern` (`^ {2}\S[^:\n]*:`) recognizes both single-line entries (`  build: ./build`) and block entries (`  build:\n    cmds: ...`) — it must stay permissive enough for both, since real includes/vars entries are often one-liners while tasks are usually blocks.

### CLI behavior

- Config file resolution priority: `-c`/`-config` flag > `CONFIG` env var > `./config.yml` > `./config.yaml` (first that exists) — see `configPathOrDefault` in `internal/cli/cli.go`.
- Templates dir resolution: `TEMPLATES` env var > `./templates`.
- A `.env` file in the working directory is loaded automatically (by `main`, before `Run`), so it's the usual way to set `CONFIG`/`TEMPLATES` per project; `.env.example` documents the two variables.
- Exit code is `0` only if every file in the config generated successfully; if any file fails it's `1`, and every file's `[OK]`/`[NG]` status is still printed individually (partial failures don't stop the rest from generating).
- `-d`/`-dryrun` builds and YAML-validates without writing to disk.
- `-debug` prints `[DEBUG]` diagnostics (resolved paths, and each output file's per-category snippet lists) to stderr.
- `-s`/`-silent` suppresses the `[OK]` lines on stdout only — `[NG]` failures still go to stderr and the exit code is unaffected. If `-debug` is also set, `-debug` wins (`silent` is forced off in `Run`, before dispatch) — the two aren't a validation error, just a priority order.
- `[NG]`/`[WARN]` status lines are ANSI-colored (red/yellow) unconditionally — no TTY detection, so piping/redirecting output will include the raw escape codes.
- `generator.GenerateAll` flags a `Result.Warning` when two or more config files share the same output `Path` (every one after the first in name-sorted order); `cli.runGenerate` prints it as a yellow `[WARN]` regardless of `-silent`, and it never contributes to the non-zero exit code — only `Result.Error` does.

### Testing conventions already in place

- `gotasky_test.go` (package `main`) is an integration test that exercises the CLI against this repo's **real** `templates/` directory — if you rename/restructure files under `templates/`, update the config content embedded in `TestGenerate_RealTemplates` to match.
- `internal/generator/testdata/templates/...` holds isolated fixtures for unit tests; add new fixture files there (not under the real top-level `templates/`) when testing generator edge cases.
- CLI tests drive `Run(...)` end-to-end rather than testing internal helpers directly.
