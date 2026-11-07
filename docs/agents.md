# Interfaces for people, scripts and agents

oh-my-toys offers every tool through three front ends that share one core:

| Front end | For | Form |
|---|---|---|
| Desktop app | People | Main window, tray panel, command palette |
| `omt` | People, scripts and agents | Command line with human-readable and `--json` output |
| `omt mcp` | AI agents | A [Model Context Protocol](https://modelcontextprotocol.io) server over stdio |

An Agent Skill, generated from the same tool descriptions, teaches agents to use `omt` directly when they prefer a shell over MCP.

The rule that makes this work: **a tool is described once, and every front end is generated from that description.** No action exists only in the UI, only in the CLI or only in MCP.

## 1. What a tool declares

On top of the manifest in CONTRIBUTING.md (id, names, category, kind, icon, keywords, platforms), every action declares:

| Field | Meaning |
|---|---|
| `name` | Stable English identifier, such as `plan`, `apply`, `start`, `list` |
| `input` | A typed schema (a JSON Schema subset: objects, strings, numbers, booleans, enums, arrays, paths) |
| `output` | A typed schema for the result |
| `risk` | `read` (changes nothing), `write` (changes files), `process` (ends or starts processes), `system` (changes system state such as sleep) |
| `idempotent` | Whether running it twice has the same effect as once |
| `summary` | One sentence, as an i18n key, shown in every front end |

From these, the core generates:

- the form and the result view in the desktop app,
- the flags, help text and `--json` output of `omt <tool> <action>`,
- the MCP tool `<tool>_<action>` with its `inputSchema`, `outputSchema` and annotations,
- the Agent Skill reference.

| Risk | MCP annotations |
|---|---|
| `read` | `readOnlyHint: true` |
| `write`, `process` | `readOnlyHint: false`, `destructiveHint: true` |
| `system` | `readOnlyHint: false`, `destructiveHint: false` |

`openWorldHint` is always `false`: tools act on this computer only.

## 2. Plan before apply

Action tools already split into `Plan` and `Apply`. For agents this split is mandatory:

1. `<tool>_plan` returns a `plan_id`, a summary and the list of changes. It changes nothing.
2. `<tool>_apply` takes only a `plan_id`. It performs exactly the listed changes and nothing else.
3. A plan expires after 10 minutes and is bound to its inputs. `apply` checks that the affected files and processes are still in the state the plan saw; if not, it fails and asks for a new plan.

## 3. Human confirmation

Actions with risk `write` or `process` need a person's approval when an agent calls them:

- When the desktop app is running, it shows a native confirmation with the plan's changes. The person can approve or reject.
- When it is not running, `omt mcp` asks through MCP elicitation, so the agent's client shows the question.
- With no way to ask, the call fails with "confirmation required"; the agent reports this instead of retrying.

People can allow a tool to run without confirmation for a given client in Settings. This is off by default.

## 4. Scope

- File tools only work inside allowed folders. Defaults: Downloads, Desktop and Documents. People can add or remove folders in Settings.
- System folders and the app's own data are always refused.
- Deleted files go to the Trash / Recycle Bin, never removed outright.

## 5. Switch tools

`start`, `stop` and `status` of switch tools (such as Keep awake) go to the running desktop app over local IPC, so state stays in one place. The tray panel marks a tool started by an agent with the client's name until it stops. Without the desktop app, `omt` runs the switch tool itself for as long as the command runs.

A switch tool that can run for a set time (Keep awake) takes two optional inputs on `start`, one at most: `for`, a length such as `90m` or `1h30m` (1 second to 99 hours 59 minutes), and `until`, a time of day such as `18:30` in 24 hours, which is tomorrow's when today's has passed. On the command line they are `--for` and `--until`. A start with neither uses the settings used last. Without the desktop app, `omt` holds the tool for that length and then exits by itself.

## 6. Activity log

Every call from an agent is recorded locally: time, client name, tool, action, inputs, result. The log is visible in the desktop app under Settings and never leaves the computer.

## 7. The command line contract

So that scripts and agents can rely on it:

- `--json` prints one JSON document following the action's output schema. Its shape only changes with a major version.
- Exit codes: `0` success, `1` error, `2` usage error, `3` confirmation required, `4` not permitted (outside scope or refused).
- When standard input is not a terminal, `omt` never prompts. An action that needs confirmation exits with code `3` unless it was approved in the desktop app.
- `omt <tool> plan` and `omt <tool> apply --plan <id>` mirror the MCP tools.

## 8. Transport

- `omt mcp` speaks MCP over stdio, using the official Go SDK ([modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)), which needs no cgo.
- No network listener, no cloud service and no telemetry.
- Setup is one entry in the agent client's configuration, such as `{"command": "omt", "args": ["mcp"]}`.

## 9. Rollout

1. Add typed `input`, `output` and `risk` to every action. New tools are written this way from the start.
2. Ship `omt` with `--json`, exit codes and plan / apply.
3. Ship `omt mcp` with `read` tools only (Color, the port list).
4. Add confirmation, scope and the activity log; then expose `write`, `process` and `system` tools.
5. Generate and publish the Agent Skill.
