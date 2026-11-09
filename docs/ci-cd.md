# CI and releases

The workflows are in `.github/workflows`: `ci.yml`, `pr-title.yml`,
`release-please.yml` and `release.yml`. This document says what they do and
what to set up before the first release.

## Overview

1. A pull request runs **CI**. It must pass before merging.
2. Merging to `main` updates a **release PR** that bumps the version and the changelog from the commit messages.
3. Merging the release PR tags `vX.Y.Z`, creates a **draft** release and runs the **release** workflow, which builds, signs and uploads the desktop app and `omt` for every platform to that draft, then publishes it. Installed apps update themselves from it.

Nobody edits version numbers or the changelog by hand: release-please writes
both into the release PR.

## 1. CI (`.github/workflows/ci.yml`)

Runs on pull requests that are ready for review: a draft runs nothing until
it is marked ready, and a newer push cancels the run of the older one. It does
not run on `main`, which receives squash merges of the pull requests whose last
run checked the tree that lands there. The pull request's title, which
becomes the squash-merge commit message, is checked by `pr-title.yml` instead,
a workflow of its own so that a push, or an edit of the title or the body,
re-runs that check alone rather than the whole of CI: the title must follow
Conventional Commits with the types of
CONTRIBUTING.md section 13, with an English, imperative summary that starts
with a lowercase letter, has no trailing period and is at most 72 characters.

| Job | Runner | What it checks |
|---|---|---|
| Lint | ubuntu, once per `GOOS` (linux, darwin, windows) | `gofumpt` and `goimports` with the local prefix (on linux), `go vet`, and `golangci-lint` with the linters in CONTRIBUTING.md section 4, including `depguard` for the layers. The per-OS code under `internal/platform` is split by build tags, so each OS is checked on its own. |
| Test | ubuntu, macOS, Windows | `go test -race ./...`, with cgo as the race detector needs it, then `go test ./...` with `CGO_ENABLED=0`, as the programs ship. |
| Live | ubuntu, Windows | The live tests under `internal/platform`, run with `OMT_LIVE=1` (without it they skip, so `go test ./...` never touches a developer's system). They really change the runner's proxy, trust store and network capture and check the result with the system's own tools: the Windows registry and `certutil`, `update-ca-certificates` and the system bundle, `gsettings`, and a raw socket on the loopback. With `OMT_LIVE=1` a missing prerequisite fails the test, it never skips it. On Windows, `certutil` asks for a confirmation when a root certificate is added to or removed from the user's store; `.github/scripts/approve-root-prompt.ps1` runs beside the tests and answers Yes, as a person would. macOS has no live job: its trust and proxy changes ask for an administrator password that a runner cannot give. |
| E2E | macOS, Windows, ubuntu | The real app in a real window, built with the `e2e` tag and driven through MyGo's automation plugin the way a screen reader drives it (`dev/e2e`). It checks the sidebar, Keep Awake, Ports and Color, then the Traffic scenario: start a capture, send a request with the real `curl`, see the flow named for `curl` in the list, filter it away and back, open Settings, stop; and it checks that the toolbar does not move while the capture runs. The app starts with its own settings folder, system proxy and decryption off, so the runner's network is untouched. Ubuntu has no GPU and no display: Xvfb provides one and the UI draws in software. The run uploads PNGs of the window (`e2e-<os>` artifacts). |
| Build | ubuntu | `CGO_ENABLED=0` cross-builds of every package for darwin, windows and linux on amd64 and arm64. |
| i18n | ubuntu | `go tool mygo i18n check` reports no errors, and no warning either, so every message in `en.json` exists in `zh-Hans.json`; then `go tool mygo generate` and `go generate ./internal/ui/ds` leave `internal/` unchanged, so the generated catalogs and tokens are committed with their sources. |
| Licences | ubuntu, once per `GOOS` | `go-licenses` finds only the licences allowed by CONTRIBUTING.md section 3. |

`setup-go` restores the module and build caches between runs. The MyGo CLI is
the tool `go.mod` pins, so nothing is installed for it. The versions of the
tools CI adds (`golangci-lint`, `gofumpt`, `goimports`, `go-licenses`) are
pinned in the `env` of the workflow: a new release of a tool must not turn a
green branch red, so they are bumped by hand or by Dependabot.

`github.com/go-text/typesetting` is dual licensed under the Unlicense and
BSD-3-Clause; the app uses it under the BSD-3-Clause, so the licence job
ignores the module instead of allowing the Unlicense.

## 2. Release PR (`.github/workflows/release-please.yml`)

Runs on every push to `main`. [release-please](https://github.com/googleapis/release-please)
reads the Conventional Commits merged since the last release and keeps one
open release PR:

- `fix` raises the patch version, `feat` the minor version, a `!` or `BREAKING CHANGE:` the major version (the minor one while the version is below 1.0).
- The PR updates `CHANGELOG.md` and the version in `mygo.json`. The configuration is in `release-please-config.json`, the last released version in `.release-please-manifest.json`.
- Merging it creates the tag `vX.Y.Z` and a draft GitHub Release, whose notes are the changelog section of the version.

With the manifest at 0.1.0, the version in `mygo.json` today, and the `feat`
commits merged so far, the first release PR is titled
`chore(main): release 0.2.0`.

**A pull request and a tag made with the workflow's own token start no
workflows.** Without `RELEASE_PLEASE_TOKEN`, the release PR gets no CI run, so
it cannot satisfy a branch protection rule that requires CI, and the release
workflow would not run for its tag. Add a fine-grained personal access token
(contents and pull requests: read and write) or a GitHub App token as the
secret `RELEASE_PLEASE_TOKEN`. The release itself is called from this workflow
rather than triggered by the tag, so it runs either way.

## 3. Release (`.github/workflows/release.yml`)

It takes the tag as an input: `release-please.yml` calls it after creating a
release, and it can be run by hand (`workflow_dispatch`) to build a tag again;
its files replace the ones already on the draft.

| Job | Runner | Output |
|---|---|---|
| Draft release | ubuntu | Checks that the tag matches `version` in `mygo.json`, so a wrong tag builds nothing, and creates the draft if the tag was pushed by hand. The draft exists before the builds start, so each of them uploads to it instead of making a draft of its own. |
| macOS | macos-latest | Universal `.app` in a `.dmg`, signed and notarized; update archive |
| Windows | ubuntu-latest (with NSIS and `osslsigncode`) | `.exe` and installer for amd64 and arm64; update archive |
| Linux | ubuntu-latest | `.tar.gz` with install script, `.deb`, `PKGBUILD` for amd64 and arm64; update archive |
| CLI | ubuntu-latest | `omt` `.tar.gz` for darwin and linux and `.zip` for windows, on amd64 and arm64, with `omt-SHA256SUMS` |
| Publish | ubuntu | Marks the draft as a release, or as a pre-release when the macOS app is not signed. |

The app jobs run `go tool mygo build -platform … -upload`, which builds,
signs, makes the installers and the update archives and uploads them to the
draft; the CLI job builds `omt` with `go build` and uploads its archives with
`gh`.

Signing is configured in a copy of `mygo.json` made on the runner, so a build
on a developer's machine and a release without the secrets stay ad hoc
signed. Without the Apple certificate, Gatekeeper blocks the app on other
Macs, so the publish job marks such a release a pre-release: installed apps
skip pre-releases, so a build that is not signed for other Macs does not
reach them (the app does not opt into pre-releases). Without the Windows
certificate, SmartScreen warns on first launch.

The desktop app and `omt` share one version number and one release. The app
checks `cxxzsh/oh-my-toys` releases for updates (MyGo `updates.github`),
verifies the signature with the public key in `mygo.json`, and installs them;
MyGo also makes delta updates from earlier versions. The update window shows
the section of `CHANGELOG.md` that `mygo build` puts in the manifests.

## Secrets

Stored as GitHub Actions secrets. None of them is ever committed.

| Secret | Needed for | Notes |
|---|---|---|
| `RELEASE_PLEASE_TOKEN` | `release-please.yml` | A personal access token or GitHub App token, so that the release PR runs CI. See section 2. |
| `MYGO_UPDATER_PRIVATE_KEY` | Signing updates | Created with `mygo keygen`, the contents of `mygo-update.key`. Losing it strands installed apps; leaking it lets anyone ship code to users. Keep a copy in a password manager. |
| `MACOS_CERTIFICATE` | Signing the macOS app | Base64 of a "Developer ID Application" certificate and its private key exported from Keychain Access as a `.p12`. Requires the Apple Developer Program (paid, yearly). |
| `MACOS_CERTIFICATE_PASSWORD` | Signing the macOS app | The password of the `.p12`. |
| `APPLE_API_KEY` | Notarizing the macOS app | Base64 of the App Store Connect API key file (`AuthKey_XXXXXXXXXX.p8`), with its key ID and issuer in the next two. |
| `APPLE_API_KEY_ID` | Notarizing the macOS app | The key's ID. |
| `APPLE_API_ISSUER` | Notarizing the macOS app | The issuer UUID of the API key. |
| `WINDOWS_CERTIFICATE` | Signing the Windows app | Base64 of the code signing `.pfx`. Without it, Windows SmartScreen warns on first launch; it can be added later. |
| `WINDOWS_CERTIFICATE_PASSWORD` | Signing the Windows app | The password of the `.pfx`. |

```sh
base64 -i certificate.p12 | gh secret set MACOS_CERTIFICATE
gh secret set MACOS_CERTIFICATE_PASSWORD
base64 -i AuthKey_XXXXXXXXXX.p8 | gh secret set APPLE_API_KEY
gh secret set APPLE_API_KEY_ID
gh secret set APPLE_API_ISSUER
gh secret set MYGO_UPDATER_PRIVATE_KEY < mygo-update.key
base64 -i code-signing.pfx | gh secret set WINDOWS_CERTIFICATE
gh secret set WINDOWS_CERTIFICATE_PASSWORD
gh secret set RELEASE_PLEASE_TOKEN
```

## Repository settings

- `main` is protected: changes arrive through pull requests, CI must pass, force pushes are blocked and history must be linear (no merge commits).
- Only squash merging is allowed, so each pull request becomes one Conventional Commit.
- Branches are deleted after merging.
- Dependabot opens weekly pull requests for Go modules and GitHub Actions, in `.github/dependabot.yml`. Go modules except MyGo come as one pull request and MyGo alone, as CONTRIBUTING.md section 3 requires. Their commits are `build(deps)` and `ci(deps)`.
- Actions are allowed to create pull requests and to write to the repository (Settings → Actions → General → Workflow permissions): release-please needs both.

## First release

1. Add the secrets above; at least `MYGO_UPDATER_PRIVATE_KEY`, `MACOS_CERTIFICATE`, `MACOS_CERTIFICATE_PASSWORD`, `APPLE_API_KEY`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER` and `RELEASE_PLEASE_TOKEN` for a signed macOS release.
2. Push to `main`; release-please opens `chore(main): release 0.2.0`.
3. Merge it; the release workflow builds and publishes `v0.2.0`.

## Later

- Homebrew cask and formula, Scoop and winget manifests, and the AUR package, updated by the release workflow.
- A nightly build from `main` for testers.
- End-to-end tests of the desktop app (`dev/e2e`) on the macOS runner.
