# Contributing to oh-my-toys

These are the rules every change follows, whether a person or a coding agent writes it. When a rule and a shortcut disagree, the rule wins; when a rule is wrong, change this file in its own commit first.

## 1. Architecture

oh-my-toys is a toolbox: many small tools, one app. The code is in layers, and **a layer only depends on the layers below it**.

| Layer | What it holds | May import |
|---|---|---|
| Entry | `cmd/oh-my-toys` (desktop app), `cmd/omt` (command line) | everything below |
| UI | MyGo views: main window, tray panel, command palette, settings | core, tool manifests, i18n |
| Core | tool registry, search index, state and events, runner, settings store | tools, platform, i18n |
| Tools | one package per tool: manifest + logic, no UI | platform, i18n |
| Platform | per-OS system calls (power, ports, clipboard, OCR…) | standard library only |

- The UI never names a specific tool. Everything it shows comes from the registry.
- A tool never imports the UI, the core or another tool.
- The desktop app and `omt` are two programs that share the core, tools and platform packages. When the desktop app is running, `omt` hands commands for switch tools (such as `omt awake on`) to it over local IPC, so there is one source of state.

## 2. Repository layout

```
cmd/oh-my-toys/       desktop app entry
cmd/omt/              command line entry
internal/core/        registry, search, state, runner, settings
internal/tools/<id>/  one directory per tool; <id> is also the CLI subcommand
internal/ui/          MyGo views and design tokens
internal/platform/    per-OS code, split by build tags
internal/i18n/        catalogs: en.json (source), zh-Hans.json
spikes/               throwaway prototypes; never imported by other code
docs/                 design notes and decisions
```

## 3. Go

- Go version: the one in `go.mod`; at least what MyGo requires.
- **No cgo.** Build with `CGO_ENABLED=0` everywhere. Native APIs are called the way MyGo does: `purego` on macOS and Linux, `syscall` on Windows.
- Format with `gofumpt` (a stricter `gofmt`) and `goimports`. Lint with `golangci-lint` using the linters in section 4. CI fails on any formatting difference or lint error.
- `go vet` and `go test -race ./...` pass before every commit.
- An `.editorconfig` fixes the basics for every file: UTF-8, LF line endings, a final newline, no trailing whitespace.
- Keep dependencies few. A new module needs a reason in the PR description and a licence compatible with MIT (MIT, BSD, Apache-2.0, ISC, MPL-2.0 for unmodified use). Pin versions; MyGo is pinned to an exact version and upgraded in its own PR.

## 4. Clean code

Code is read far more often than it is written. Keep it small, plain and obvious, and let the tools hold the line.

**Enforced by linters** (`golangci-lint`; a `//nolint` needs the linter's name and a reason, checked by `nolintlint`):

| Concern | Linters | Limit |
|---|---|---|
| Correctness | `errcheck`, `govet`, `staticcheck`, `ineffassign`, `bodyclose`, `contextcheck`, `noctx`, `errorlint`, `exhaustive` | — |
| Dead code | `unused`, `unparam` | nothing unused stays |
| Size and complexity | `funlen`, `gocognit`, `cyclop`, `nestif` | 60 lines or 40 statements per function; cognitive complexity 15; nesting depth 3 |
| Duplication | `dupl`, `goconst` | — |
| Style | `revive`, `gocritic`, `nakedret`, `misspell`, `predeclared`, `prealloc` | — |
| Layers | `depguard` | the table in section 1 |
| Security | `gosec` | — |

**Rules the linters cannot fully check** (reviewers check them):

- A function does one thing; its name says what. A file holds one idea and stays under about 400 lines.
- Return early. No `else` after a `return`. Keep the happy path at the left edge.
- No magic numbers or strings: name them as constants, or take them from the design tokens and i18n catalogs.
- No dead code, no commented-out code, no debugging leftovers. Delete it; git remembers.
- `TODO` only with an issue: `// TODO(#42): ...`.
- No packages called `util`, `utils`, `common`, `helpers`, `misc` or `base`. Name a package after what it provides.
- Small interfaces, declared where they are used. Accept interfaces, return concrete types.
- No `init()` and no package-level variables other than constants, sentinel errors and compiled regular expressions. Tools are registered in one explicit list, not by side effects.
- No boolean parameters that change what a function does; use two functions or an options struct.
- Make the zero value useful, or provide a constructor that validates its input.
- Comments explain why, not what. If the code needs a comment to say what it does, rename or split it.
- Leave the code cleaner than you found it, but in a separate commit: a feature commit contains no unrelated clean-ups, and a refactor commit changes no behaviour.

**Review checklist**: Does each function do one thing? Could a name be clearer? Is anything duplicated, unused or commented out? Is every error handled or returned with context? Is every user-visible string in the i18n catalogs? Do the tests read as clearly as the code?

## 5. Naming

- Packages: short, lowercase, one word, no underscores (`rename`, `dupes`, `awake`).
- A tool's id, its package name and its `omt` subcommand are the same word.
- Files: lowercase with underscores; per-OS code uses the Go suffixes `_darwin.go`, `_windows.go`, `_linux.go`.
- Exported names say what they are without repeating the package: `rename.Plan`, not `rename.RenamePlan`.

## 6. Tools

Every tool provides a manifest and implements one of two contracts.

- **Manifest**: id, name and summary (as i18n keys), category, kind (`switch` or `action`), Lucide icon name, search keywords, quick actions, supported platforms.
- **Switch tools** (keep running, like Keep awake): `Start`, `Stop`, `Status`. State lives in the core, not in the tool.
- **Action tools** (run once, like Batch rename): `Plan` computes what would happen and changes nothing; `Apply` performs exactly what a plan lists and nothing else. The command line shows the plan by default and applies it only with `--apply`.
- A tool must work without the UI: the CLI and tests call it directly.
- Adding a tool never requires a change in the UI or another tool, only a registration line.

## 7. Errors, context and concurrency

- Return errors; never panic for something that can happen in normal use. Wrap with `fmt.Errorf("open %s: %w", path, err)`. Error strings are lowercase English without a trailing period; they are for logs, not for people.
- What people see comes from i18n messages, chosen from the error (sentinel errors or `errors.As`).
- Every operation that can take time takes a `context.Context` as its first parameter and stops when it is cancelled.
- Every goroutine has an owner and a way to stop. No package-level mutable state.
- The UI thread never waits on I/O. Long work goes through the core runner, which recovers panics, applies timeouts and reports progress.

## 8. Internationalisation

- No user-visible string literals in Go code. Every message is a key in `internal/i18n/en.json` (the source language) with a description, and the same key in `zh-Hans.json`, **in the same commit**.
- Use ICU MessageFormat plurals (`{count, plural, one {# file} other {# files}}`), never `file(s)`.
- Format dates, times, numbers and sizes through the i18n package, never by hand.
- Log messages, error strings, CLI flags and subcommands stay in English and are not translated. `--json` output is never translated.
- Check layouts in English: it is usually 1.5 to 2 times longer than Chinese.

## 9. UI

- Follow the design system: colours, type, spacing, radii and components come from its tokens and rules, never from literal values in a view.
- Colours are assigned per category, not per tool. Icons come from Lucide.
- Every control has an accessible name. Check both light and dark themes.

## 10. Performance budgets

| What | Budget |
|---|---|
| Tray panel opens after a click | < 100 ms |
| Command palette results after a keystroke (500 tools) | < 50 ms |
| Idle CPU | ~0% |
| Startup | reads manifests only; a tool initialises on first use |

Lists longer than 60 items are virtualised. A change that misses a budget needs a benchmark and a note in the PR.

## 11. Tests

- Tools have table-driven unit tests that run without a window.
- CLI output is checked with golden files, in both languages.
- UI views are tested headless with MyGo's view tests where possible.
- A bug fix comes with a test that fails without it.

## 12. Comments and documentation

- Code comments and identifiers are in English. Every exported identifier has a doc comment that starts with its name.
- Repository documentation is written in English. The one exception is the README, which has an English and a Simplified Chinese version (`README.md`, `README.zh-CN.md`) that change together in the same commit.

## 13. Commits and pull requests

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): summary`.
  - Types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
  - Scope: a tool id (`rename`, `awake`) or a layer or area (`core`, `ui`, `cli`, `platform`, `i18n`, `spike`, `deps`). Optional, but preferred.
  - Summary: English, imperative, lowercase, no trailing period, at most 72 characters. Example: `feat(rename): show conflicts in the preview`.
  - A breaking change adds `!` after the type or scope and a `BREAKING CHANGE:` footer.
- One logical change per commit. Formatting-only changes go in their own commit.
- Branches: `<type>/<short-topic>`, such as `feat/tray-panel`.
- Pull requests are squash-merged; the PR title becomes the commit message, so it follows the same format.
- Never force-push to `main`.

## 14. CI and releases

Every pull request runs CI; merging to `main` keeps a release PR up to date, and merging that release PR publishes a release for every platform. The details are in [docs/ci-cd.md](docs/ci-cd.md). Because releases and changelogs are generated from commit messages, the commit format in section 13 is enforced on pull request titles.

## 15. Security

This repository is public.

- Never commit secrets, tokens, signing keys, personal paths or private links.
- Never commit build output or large binaries; release artefacts go to GitHub Releases.
- Tools that change files or end processes always preview first and ask before destructive steps.
