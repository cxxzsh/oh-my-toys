# Contributing to oh-my-toys

English | [简体中文](CONTRIBUTING.zh-CN.md)

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
- Format with `gofmt` and `goimports`. Lint with `golangci-lint`, including `depguard` rules that enforce the layering table above. CI fails on any lint error.
- `go vet` and `go test -race ./...` pass before every commit.
- Keep dependencies few. A new module needs a reason in the PR description and a licence compatible with MIT (MIT, BSD, Apache-2.0, ISC, MPL-2.0 for unmodified use). Pin versions; MyGo is pinned to an exact version and upgraded in its own PR.

## 4. Naming

- Packages: short, lowercase, one word, no underscores (`rename`, `dupes`, `awake`).
- A tool's id, its package name and its `omt` subcommand are the same word.
- Files: lowercase with underscores; per-OS code uses the Go suffixes `_darwin.go`, `_windows.go`, `_linux.go`.
- Exported names say what they are without repeating the package: `rename.Plan`, not `rename.RenamePlan`.

## 5. Tools

Every tool provides a manifest and implements one of two contracts.

- **Manifest**: id, name and summary (as i18n keys), category, kind (`switch` or `action`), Lucide icon name, search keywords, quick actions, supported platforms.
- **Switch tools** (keep running, like Keep awake): `Start`, `Stop`, `Status`. State lives in the core, not in the tool.
- **Action tools** (run once, like Batch rename): `Plan` computes what would happen and changes nothing; `Apply` performs exactly what a plan lists and nothing else. The command line shows the plan by default and applies it only with `--apply`.
- A tool must work without the UI: the CLI and tests call it directly.
- Adding a tool never requires a change in the UI or another tool, only a registration line.

## 6. Errors, context and concurrency

- Return errors; never panic for something that can happen in normal use. Wrap with `fmt.Errorf("open %s: %w", path, err)`. Error strings are lowercase English without a trailing period; they are for logs, not for people.
- What people see comes from i18n messages, chosen from the error (sentinel errors or `errors.As`).
- Every operation that can take time takes a `context.Context` as its first parameter and stops when it is cancelled.
- Every goroutine has an owner and a way to stop. No package-level mutable state.
- The UI thread never waits on I/O. Long work goes through the core runner, which recovers panics, applies timeouts and reports progress.

## 7. Internationalisation

- No user-visible string literals in Go code. Every message is a key in `internal/i18n/en.json` (the source language) with a description, and the same key in `zh-Hans.json`, **in the same commit**.
- Use ICU MessageFormat plurals (`{count, plural, one {# file} other {# files}}`), never `file(s)`.
- Format dates, times, numbers and sizes through the i18n package, never by hand.
- Log messages, error strings, CLI flags and subcommands stay in English and are not translated. `--json` output is never translated.
- Check layouts in English: it is usually 1.5 to 2 times longer than Chinese.

## 8. UI

- Follow the design system: colours, type, spacing, radii and components come from its tokens and rules, never from literal values in a view.
- Colours are assigned per category, not per tool. Icons come from Lucide.
- Every control has an accessible name. Check both light and dark themes.

## 9. Performance budgets

| What | Budget |
|---|---|
| Tray panel opens after a click | < 100 ms |
| Command palette results after a keystroke (500 tools) | < 50 ms |
| Idle CPU | ~0% |
| Startup | reads manifests only; a tool initialises on first use |

Lists longer than 60 items are virtualised. A change that misses a budget needs a benchmark and a note in the PR.

## 10. Tests

- Tools have table-driven unit tests that run without a window.
- CLI output is checked with golden files, in both languages.
- UI views are tested headless with MyGo's view tests where possible.
- A bug fix comes with a test that fails without it.

## 11. Comments and documentation

- Code comments and identifiers are in English. Every exported identifier has a doc comment that starts with its name.
- User-facing documentation (README, user guides) is in English and Simplified Chinese, and both are updated in the same commit.

## 12. Commits and pull requests

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): summary`.
  - Types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`.
  - Scope: a tool id (`rename`, `awake`) or a layer or area (`core`, `ui`, `cli`, `platform`, `i18n`, `spike`, `deps`). Optional, but preferred.
  - Summary: English, imperative, lowercase, no trailing period, at most 72 characters. Example: `feat(rename): show conflicts in the preview`.
  - A breaking change adds `!` after the type or scope and a `BREAKING CHANGE:` footer.
- One logical change per commit. Formatting-only changes go in their own commit.
- Branches: `<type>/<short-topic>`, such as `feat/tray-panel`.
- Pull requests are squash-merged; the PR title becomes the commit message, so it follows the same format.
- Never force-push to `main`.

## 13. Security

This repository is public.

- Never commit secrets, tokens, signing keys, personal paths or private links.
- Never commit build output or large binaries; release artefacts go to GitHub Releases.
- Tools that change files or end processes always preview first and ask before destructive steps.
