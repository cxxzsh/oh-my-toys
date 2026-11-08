# oh-my-toys

English | [简体中文](README.zh-CN.md)

A box of small, handy tools for your desktop. Each tool does one thing, and they all live in one app: a main window, a menu bar / tray panel and a command palette. A standalone command line program, `omt`, runs the same tools from a terminal. For macOS, Windows and Linux.

## Status

The app and `omt` are in this repository. Run the desktop app with `go run ./cmd/oh-my-toys` and the command line with `go run ./cmd/omt`. macOS has been tried. Windows and Linux packages can be built, but have not been run on those systems. There is no signed release yet.

On macOS, Cmd+Shift+A takes a screenshot. A magnifier follows the pointer while you drag, and hovering a window outlines it, so a click takes the whole window. After you release, the toolbar sits on the selection’s bottom right: draw arrows, mosaics and text, then Copy, Save or Cancel. Enter copies, Cmd+S saves a PNG under ~/Pictures/oh-my-toys, Esc and a right click step back, and Cmd+Z undoes an annotation. Screenshot is one of the tools: open its page to start a capture or change the folder. The same command is in the Tools menu.

## Planned tools

- **Keep awake**: stop the computer from sleeping, with a preset, a length or a time of day you type in, or a daily plan of time spans that turns it on and off by itself.
- **Batch rename**: rename files by rules; see a preview before anything changes.
- **Find duplicates**: find files with the same content and how much space they take.
- **Ports**: see which process holds a port, and free it.
- **Color**: convert between color formats and check text contrast.

More tools will follow. The app is designed to hold many of them: search, pinned tools and categories keep it fast to find the one you need.

## Languages

The app and its documentation come in English and Simplified Chinese.

## For scripts and AI agents

Every tool that works without a screen is also available from the command line (`omt`, with `--json` output) and as an MCP server (`omt mcp`) for AI agents. `omt mcp` exposes read actions and switch tools. Actions that change files or end processes are not exposed yet. Screenshot stays in the desktop app, which needs the screen and the person in front of it. See [docs/agents.md](docs/agents.md).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for the code conventions and commit format.

## License

[MIT](LICENSE)
