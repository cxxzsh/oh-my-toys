# oh-my-toys

English | [简体中文](README.zh-CN.md)

A box of small, handy tools for your desktop. Each tool does one thing, and they all live in one app: a main window, a menu bar / tray panel and a command palette. A standalone command line program, `omt`, runs the same tools from a terminal. For macOS, Windows and Linux.

## Status

oh-my-toys is being rewritten from scratch. The UI framework ([MyGo](https://github.com/egoist/mygo), Go) is being validated before the app is built, so there is nothing to install yet.

## Planned tools

- **Keep awake**: stop the computer from sleeping, for a set time or until you turn it off.
- **Batch rename**: rename files by rules; see a preview before anything changes.
- **Find duplicates**: find files with the same content and how much space they take.
- **Ports**: see which process holds a port, and free it.
- **Color**: convert between color formats and check text contrast.

More tools will follow. The app is designed to hold many of them: search, pinned tools and categories keep it fast to find the one you need.

## Languages

The app and its documentation come in English and Simplified Chinese.

## For scripts and AI agents

Every tool is also available from the command line (`omt`, with `--json` output) and, planned, as an MCP server (`omt mcp`) for AI agents, with a preview before any change and a person's confirmation for anything that modifies files or processes. See [docs/agents.md](docs/agents.md).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the code conventions and commit format.

## License

[MIT](LICENSE)
