# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gotasky` is a Go CLI that generates one or more `Taskfile.yml` files (for [go-task](https://taskfile.dev)) by combining reusable snippet files. Instead of copy-pasting the same boilerplate (cleanup tasks, timing wrappers, OS-update tasks, ...) across many projects, you keep each snippet once under `templates/` and declare per-output-file which snippets to combine in a YAML config.

## Commands

This repo uses [go-task](https://taskfile.dev) (`Taskfile.yml`) for its own build/test, not raw `go` commands directly, though both work. Task logic itself lives in shared remote Taskfiles (`sig9org/tasks` on GitHub, pulled in via `includes:` as `_cleanup`/`_go`/`_time`); this repo's `Taskfile.yml` just wraps them with project-specific names/aliases:

```sh
task                     # list all available tasks (default)
task go-build (gb)       # build ./dist/gotasky for the current platform
task go-all-build (ga)   # cross-compile release binaries for all platforms into ./dist
task go-test (gt)        # go vet ./... && go test ./...
task go-clean (gc)       # empty ./dist
task go-register (gr)    # register the latest tagged version on pkg.go.dev
task go-version (gv)     # print version info (builds and runs with -v)
task cleanup (c)         # remove editor/OS cruft (.DS_Store, .terraform, ...)
```

Equivalent raw Go commands:

```sh
go vet ./...
go test ./...
go test ./... -run TestName          # single test across all packages
go test ./internal/generator/...     # single package
go build -o gotasky .
```

`BINARY_NAME`, `GITHUB_USER`, and `GITHUB_REPO` used by the shared `_go` tasks are defined in `project.ini` (loaded via `dotenv` in `Taskfile.yml`); `VERSION_PKG` (the `-ldflags -X` target) and the module path are computed from those two, not stored directly. Single-platform builds (`go-build`, `go-version`) inject a version via `git describe --tags --always` (falls back to `"dev"` with no tags); cross-compiled release builds (`go-all-build`) instead use `git describe --tags --abbrev=0` (falls back to `"v0.0.0"`) for both the injected version and the output filenames — so a release build and a same-commit dev build can carry different version strings.

## Architecture

Entry point `gotasky.go` just delegates to `internal/cli.Run(args, stdout, stderr) int`. All packages under `internal/` have a single, narrow responsibility:

- **`internal/config`** — `Load(path)` parses the YAML config file into `Config{Templates []string, Files map[string]File}`. The top-level `Templates` key is gotasky's own optional tool setting (which template directories to search); `Files` maps each output to its `Path` and, per category (`Headers`, `Includes`, `Vars`, `Tasks`), which snippet filenames to combine. `Load` explicitly rejects a top-level `config` key (probing a generic `map[string]any` before decoding into `Config`, which has no such field) — a config file naming its own path from inside itself is contradictory, so this is a hard error rather than a silently-ignored no-op.
- **`internal/generator`** — the core logic. `Generator.Build(cfg config.File)` reads snippet files from each dir in `TemplatesDirs/<category>/<name>` (first match wins) and assembles the final Taskfile body; `GenerateAll(cfg, templatesDirs, rootDir, dryRun)` does this for every file in the config, sorted by name, writing (or, in dry-run mode, just validating) each one.
- **`internal/selfupdate`** — wraps `go-selfupdate` to check GitHub releases and replace the running binary; treats a non-semver `Version` (i.e. `"dev"`, the default for local builds) as "not a release build" rather than erroring against the network.
- **`internal/cli`** — flag parsing and wiring; `Run` is the whole CLI surface, tested by driving it end-to-end (`Run([]string{...}, &stdout, &stderr)`) rather than unit-testing flag parsing in isolation.
- **`internal/version`** — a `Version` var overwritten at build time via `-ldflags "-X .../internal/version.Version=..."` (holds `git describe --tags` output, or `"dev"` for local builds), plus `Commit()` (VCS revision read from `runtime/debug.ReadBuildInfo`, embedded automatically by the Go toolchain) and `String()` (`"gotasky <version> (<commit>)"`, used by `-v` and `-h`).

### Template categories and merging (the part worth understanding before touching `generator.go`)

Templates live under `<templates-dir>/<category>/*` in four categories: `headers`, `includes`, `vars`, `tasks`. There can be more than one templates directory (see the config file's top-level `templates` key below) — they form a search path, tried in order per snippet name, so a project-specific directory can be layered on top of a shared one. For a given output file, `Build` renders `headers` snippets as-is (concatenated, verbatim — this becomes the top of the file: `version`, `silent`, `dotenv`, ...), and for `includes`/`vars`/`tasks` it:

1. Reads every named snippet for that category.
2. Runs each through `splitSections`, which reassigns lines to a different section whenever it hits a bare `vars:`, `tasks:`, or `includes:` marker at column 0 mid-file — so a snippet under `templates/tasks/` can also declare its own `vars:` block (e.g. a variable only that task needs), and it gets merged into the top-level `vars:` section rather than duplicating a `tasks:` key or corrupting the tasks section. Content before the first such marker (or all of it, if there's none) is attributed to the snippet's own category folder.
3. Pools the resulting chunks per section across *all* snippets/categories, then sorts entries alphabetically (case-insensitive) via `sortEntries`/`entryPattern` — `tasks:` pins `default` first, `includes:`/`vars:` are plain alphabetical.
4. Wraps each non-empty pooled section in its single top-level key (`includes:`, `vars:`, `tasks:`) and joins everything, then YAML-validates the result before writing.

This section-splitting logic is why a naive re-implementation ("just glob and concatenate the files") would silently break: two snippets that both start with `tasks:` (common if they were copy-pasted from a full Taskfile) must not produce two `tasks:` keys, and a `vars:` block embedded inside a `tasks/*.yml` snippet must land in the `vars:` section, not get parsed as a bogus task entry.

`entryPattern` (`^ {2}\S[^:\n]*:`) recognizes both single-line entries (`  build: ./build`) and block entries (`  build:\n    cmds: ...`) — it must stay permissive enough for both, since real includes/vars entries are often one-liners while tasks are usually blocks.

### CLI behavior

- Only `-update`, `-dryrun`, and `-silent` exist as long-form flags — they have no single-letter shorthand (`-u`/`-d`/`-s` were removed; each is now just an unrecognized flag). `-c`/`-config`, `-v`/`-version`, and `-h`/`-help` still have their shorthands.
- Config file resolution priority: `-c`/`-config` flag > `./config.yml` > `./config.yaml` (first that exists) — see `configPathOrDefault` in `internal/cli/cli.go`. There is no other way to point at a config file (no settings file, no env var) — the flag and the two built-in filenames are the whole story.
- Templates dir resolution: the loaded config file's top-level `templates` key (a YAML list, one or more directories, searched in order — first match per snippet wins) > `./templates`. There is no `config.ini`/`.env` — every gotasky setting lives in the one config file, hence `runGenerate` in `internal/cli/cli.go` loads the config *before* resolving `templatesDirs`, reversing the old settings-file-first order.
- Exit code is `0` only if every file in the config generated successfully; if any file fails it's `1`, and every file's `[OK]`/`[NG]` status is still printed individually (partial failures don't stop the rest from generating).
- `-dryrun` builds and YAML-validates without writing to disk.
- `-debug` prints `[DEBUG]` diagnostics (resolved paths, and each output file's per-category snippet lists) to **stdout**, each line timestamped and gray-colored.
- `-silent` suppresses the `[OK]` lines on stdout only — `[NG]` failures still go to stderr and the exit code is unaffected. If `-debug` is also set, `-debug` wins (`silent` is forced off in `Run`, before dispatch) — the two aren't a validation error, just a priority order.
- Status/diagnostic lines are ANSI-colored unconditionally — no TTY detection, so piping/redirecting output will include the raw escape codes: `[NG]` red, `[WARN]` orange, `[DEBUG]` gray, `[OK]` and other normal messages uncolored.
- `generator.GenerateAll` flags a `Result.Warning` when two or more config files share the same output `Path` (every one after the first in name-sorted order); `cli.runGenerate` prints it as an orange `[WARN]` regardless of `-silent`, and it never contributes to the non-zero exit code — only `Result.Error` does.
- `-h`/`-help` output is built by `writeFlagDocs` from the `flagDocs` table in `internal/cli/cli.go`, not hardcoded strings — it right-pads each flag's short form (fixed 4-char column) and long form (padded to the widest entry) so every row's description starts at the same column, regardless of which flags do or don't have a shorthand.

### Testing conventions already in place

- `gotasky_test.go` (package `main`) is an integration test that exercises the CLI against this repo's **real** `templates/` directory — if you rename/restructure files under `templates/`, update the config content embedded in `TestGenerate_RealTemplates` to match.
- `internal/generator/testdata/templates/...` holds isolated fixtures for unit tests; add new fixture files there (not under the real top-level `templates/`) when testing generator edge cases.
- CLI tests drive `Run(...)` end-to-end rather than testing internal helpers directly.
