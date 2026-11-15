# omnifocus-mcp

MCP server (stdio) for OmniFocus, written in Go. It runs Omni Automation JavaScript inside OmniFocus via `/usr/bin/osascript -l JavaScript` and `Application('OmniFocus').evaluateJavascript`. Replaces the Python `omnifocus-operator`. macOS only; OmniFocus must be running and the host process needs Automation permission for OmniFocus (System Settings > Privacy & Security > Automation).

## Status

| Tool | State |
| --- | --- |
| `list_perspectives` | implemented (T0) |
| `list_tasks`, `list_projects`, `list_tags`, `list_folders`, `get_task`, `get_project`, `get_tag`, `get_folder` | implemented (T1) |
| `add_tasks`, `edit_tasks` | implemented (T2) |
| `add`, `edit`, `drop`, `delete` | implemented (T3) |

## Install and configure

```sh
mise exec -- make build   # produces ./omnifocus-mcp
```

Claude Code:

```sh
claude mcp add omnifocus -- /absolute/path/to/omnifocus-mcp
```

zeroclaw (`config.toml`):

```toml
[[mcp.servers]]
name = "omnifocus"
command = "/absolute/path/to/omnifocus-mcp"
args = []
```

Tools surface as `omnifocus__<tool>`; allow read tools in unattended jobs and gate `drop_*`/`delete_*` with always-ask.

## Tool contract

Conventions: ids are OmniFocus `primaryKey` strings. Dates are ISO 8601 strings (`2026-11-18` or `2026-11-18T08:00:00`, local time when no offset). In patches, an omitted field means unchanged and an explicit `null` clears (dates, note-free fields). Batch tools apply items in order, return one result per item (`ok` or `error`), and never abort the batch on one failure. Invalid input is rejected at construction before any script runs.

### Naming decision

Per-type getters `get_task`, `get_project`, `get_tag`, `get_folder` (not one `get`). Reason: the Python server already exposes the first three, and the consumers' allowlists and prompts use those names; a per-type tool also has a precise output schema instead of a union. `get_folder` is added for symmetry.

### Read (read-only)

- `list_tasks` — filters (all optional, AND-combined): `inbox` (bool), `project` (id or name), `tag` (id or name), `available` (bool: not blocked/deferred/completed/dropped), `flagged` (bool), `completed` (bool), `due_before`, `defer_before` (dates), `search` (substring over name and note), `limit` (default 50; `null` = all). Returns tasks with id, name, note, flagged, project, parent, tags, defer/due/planned dates, completed, status.
- `list_projects` — `status` filter (`Active`, `OnHold`, `Done`, `Dropped`), optional `folder`, `search`, `limit`.
- `list_tags` — includes status and `remainingTasks`.
- `list_folders` — includes parent and status.
- `list_perspectives` — `{perspectives: [{id|null, name, builtin}]}`.
- `get_task`, `get_project`, `get_tag`, `get_folder` — `{id}`; error if not found.

### Task writes

- `add_tasks` — `{tasks: [{name, note?, flagged?, defer?, due?, planned?, project? | parent? (default inbox), tags?: [id|name]}]}`; returns `{results: [{id, ok, error?, task?}]}`, one per task in order, with the created task records. Dates `YYYY-MM-DD` or `YYYY-MM-DDTHH:MM:SS`; tags by id or unambiguous name.
- `edit_tasks` — `{patches: [{id, name?, note?, flagged?, defer?, due?, planned? (null clears), complete? | uncomplete?, move?: {project: id} | {inbox: true} | {parent: id}, tags?: {add?: [..], remove?: [..]}}]}`. `complete` and `uncomplete` are mutually exclusive; `move` takes exactly one target. `null` clears `note` and the three dates; `name` cannot be empty; a patch must change something. Returns `{results: [{id, ok, error?, task?}]}` in input order with the updated task records; an item that fails validation (bad date, empty name, contradictory move, unknown or ambiguous tag) or fails in OmniFocus (unknown id) gets `ok: false` and does not affect the others. One script call per batch (plus one tag lookup when a patch names tags).

### Structure writes

Batch tools: each takes `{items: [...]}` and returns `{results: [{id, kind, ok, error?, name?, status?, parent?}]}` in input order; an invalid or refused item gets `ok: false` and does not affect the others.

- `add` — items `{kind: project|tag|folder, name, parent? (folder for project/folder, tag for tag; omit = root), status?, sequential? (project)}`.
- `edit` — items `{kind, id, name?, status?, sequential? (project), move?: {parent: id | null}, reorder?: {before|after: id}}`. `move` and `reorder` are mutually exclusive (a reorder adopts the sibling's parent); moving an object into itself or a descendant is refused. Project status: `Active`, `OnHold`, `Done`. Tag status: `Active`, `OnHold` (`Active` re-activates a dropped tag). Folders: rename, move, reorder only.

### Destructive (separate names so clients can gate them)

Items `{kind: task|project|tag|folder, id}`; same result shape. Guards run in the script (it alone can see the counts) and surface as that item's `error`.

- `drop` — Reversible. A tag may be dropped only when `remainingTasks == 0`.
- `delete` — Irreversible. Tags and folders only when already dropped and empty (children first). Tasks and projects may be deleted directly.

## Architecture

- `internal/omni`: `Bridge.Call(ctx, script, args, &out)` composes `JSON.stringify((<js/script.js>)(JSON.parse(<json string literal>)))`, runs it through the `Runner` interface, decodes JSON. Per-call context timeout (default 30 s). Errors distinguish OmniFocus not running and missing Automation permission.
- `internal/server`: tool registration and input/output types.
- Tests fake `Runner`; `make live` (`-tags live`) runs read-only checks against the real app.
