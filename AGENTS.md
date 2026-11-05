# Guide for coding agents

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing anything; it is the source of truth. The rules agents most often get wrong:

- **Layers**: entry → UI → core → tools → platform. A layer imports only the layers below it. The UI never names a specific tool; tools never import the UI, the core or each other.
- **Two programs**: `cmd/oh-my-toys` (desktop, MyGo) and `cmd/omt` (command line) share `internal/core`, `internal/tools` and `internal/platform`.
- **No cgo**: everything builds with `CGO_ENABLED=0`.
- **Tools**: a manifest plus either `Start` / `Stop` / `Status` (switch) or `Plan` / `Apply` (action). `Plan` never changes anything. Every action declares typed `input` / `output` and a `risk`; UI, CLI and MCP are generated from them (see `docs/agents.md`).
- **i18n**: no user-visible string literals. Add every message to `internal/i18n/en.json` and `zh-Hans.json` in the same commit; use ICU plurals.
- **Docs**: write repository docs in English. Only the README is bilingual: `README.md` and `README.zh-CN.md` change together.
- **Commits**: Conventional Commits, `type(scope): summary`, English, imperative, lowercase, no trailing period. Never force-push `main`.
- **CI**: see `docs/ci-cd.md`; pull request titles must follow the commit format.
- **Public repository**: never commit secrets, personal paths or private links.
- **Clean code**: follow section 4 of CONTRIBUTING.md strictly: small functions (≤ 60 lines, nesting ≤ 3), no dead or commented-out code, no `util`/`helpers` packages, no magic values, no `init()`, `//nolint` only with a reason. Clean-ups go in their own commits.
- **Before committing**: `gofumpt`, `golangci-lint`, `go vet` and `go test -race ./...` pass.
