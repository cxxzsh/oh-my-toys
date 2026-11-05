# CI and releases

This is the plan for continuous integration and automatic releases. The workflows are added once the project has Go code; this document is what they must do.

## Overview

1. A pull request runs **CI**. It must pass before merging.
2. Merging to `main` updates a **release PR** that bumps the version and the changelog from the commit messages.
3. Merging the release PR tags `vX.Y.Z` and runs the **release** workflow, which builds, signs and publishes the desktop app and `omt` for every platform to a GitHub Release. Installed apps update themselves from it.

Nobody edits version numbers or the changelog by hand.

## 1. CI (`.github/workflows/ci.yml`)

Runs on every pull request and on every push to `main`.

| Job | Runner | What it checks |
|---|---|---|
| PR title | ubuntu | The title follows Conventional Commits (it becomes the squash-merge commit message) |
| Lint | ubuntu | `gofumpt`, `goimports`, `go vet`, and `golangci-lint` with the linters in CONTRIBUTING.md section 4, including `depguard` for the layers |
| Test | ubuntu, macOS, Windows | `go test -race ./...` on each operating system |
| Build | ubuntu | `CGO_ENABLED=0` cross-builds of `cmd/omt` and `cmd/oh-my-toys` for darwin, windows and linux on amd64 and arm64 |
| i18n | ubuntu | Every key in `en.json` exists in `zh-Hans.json`, and the catalogs compile |
| Licences | ubuntu | Every dependency's licence is on the allowed list in CONTRIBUTING.md |

Go and the MyGo CLI are installed at the versions pinned in `go.mod`. Module and build caches are restored between runs to keep CI fast.

## 2. Release PR (`.github/workflows/release-please.yml`)

[release-please](https://github.com/googleapis/release-please) reads the Conventional Commits merged to `main` and keeps one open release PR:

- `fix` raises the patch version, `feat` the minor version, a `!` or `BREAKING CHANGE:` the major version (minor while the version is below 1.0).
- The PR updates `CHANGELOG.md` and the version in the MyGo configuration.
- Merging it creates the tag `vX.Y.Z` and a GitHub Release with the changelog as its notes.

## 3. Release (`.github/workflows/release.yml`)

Runs when a `v*` tag is pushed. Based on MyGo's own release workflow (MyGo docs: `github-actions.md`, `distribution.md`, `updates.md`).

| Job | Runner | Output |
|---|---|---|
| macOS | macos-latest | Universal `.app` in a `.dmg`, signed and notarized; update archive |
| Windows | ubuntu-latest (with NSIS) | `.exe` and installer for amd64 and arm64; update archive |
| Linux | ubuntu-latest | `.tar.gz` with install script, `.deb`, `PKGBUILD` for amd64 and arm64; update archive |
| CLI | ubuntu-latest | `omt` archives for every platform, with SHA-256 checksums |

All files go to the release created in step 2. The desktop app checks that release for updates (MyGo `updates.github`), verifies the signature with the public key built into the app, and installs it; MyGo also produces delta updates from earlier versions.

The desktop app and `omt` share one version number and one release.

## Secrets

Stored as GitHub Actions secrets. None of them is ever committed.

| Secret | Needed for | Notes |
|---|---|---|
| `MYGO_UPDATER_PRIVATE_KEY` | Signing updates | Created with `mygo keygen`. Losing it strands installed apps; leaking it lets anyone ship code to users. Keep a copy in a password manager. |
| Apple Developer ID certificate and password | Signing the macOS app | Requires the Apple Developer Program (paid, yearly). |
| App Store Connect API key | Notarizing the macOS app | Same account. |
| Windows code signing (optional) | Signing the Windows installer | Without it, Windows SmartScreen warns on first launch. Can be added later. |

Until the Apple and Windows signing secrets exist, releases are built unsigned and marked as pre-releases.

## Repository settings

- `main` is protected: changes arrive through pull requests, CI must pass, force pushes are blocked and history must be linear (no merge commits).
- Only squash merging is allowed, so each pull request becomes one Conventional Commit.
- Branches are deleted after merging.
- Dependabot opens weekly pull requests for Go modules and GitHub Actions.

## Later

- Homebrew cask and formula, Scoop and winget manifests, and the AUR package, updated by the release workflow.
- A nightly build from `main` for testers.
