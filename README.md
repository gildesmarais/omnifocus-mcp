# omnifocus-mcp

An [MCP](https://modelcontextprotocol.io) server that lets AI assistants read and edit your [OmniFocus](https://www.omnigroup.com/omnifocus) database: list and search tasks, add and edit tasks in batches, and manage projects, tags and folders.

It is a single Go binary speaking MCP over stdio. Each tool call runs an Omni Automation script inside OmniFocus through `osascript`, so it works with your local database and needs no account, sync server or API key.

## Requirements

- macOS with OmniFocus 4 installed and running (OmniFocus 3 with Omni Automation may work but is untested).
- Go 1.27 or newer to build it (`go install` fetches the toolchain if yours is older).
- Automation permission: the first tool call makes macOS ask whether the host app (your terminal, Claude Desktop, …) may control OmniFocus. Allow it. To change it later, go to System Settings > Privacy & Security > Automation.

## Install

```sh
go install github.com/gildesmarais/omnifocus-mcp@latest
```

The binary lands in `$(go env GOPATH)/bin/omnifocus-mcp`, usually `~/go/bin/omnifocus-mcp`. MCP clients need the absolute path.

Or build from a clone:

```sh
git clone https://github.com/gildesmarais/omnifocus-mcp.git
cd omnifocus-mcp
make build   # produces ./omnifocus-mcp
```

## Configure a client

Claude Code:

```sh
claude mcp add omnifocus -- "$(go env GOPATH)/bin/omnifocus-mcp"
```

Claude Desktop (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "omnifocus": {
      "command": "/Users/you/go/bin/omnifocus-mcp"
    }
  }
}
```

Any other MCP client: run the binary as a stdio server with no arguments.

### Safety

The server can change and delete data in OmniFocus. Read tools are safe to allow without prompting. Have your client ask before every `drop` and `delete` call. They are separate tools so you can gate them by name. Both refuse risky cases (see [Destructive](#destructive)), and `delete` cannot be undone. Back up your database before trying it on a library you care about.

## Tools

| Group | Tools |
| --- | --- |
| Read | `list_tasks`, `list_projects`, `list_tags`, `list_folders`, `list_perspectives`, `get_task`, `get_project`, `get_tag`, `get_folder` |
| Task writes | `add_tasks`, `edit_tasks` |
| Structure writes | `add`, `edit` (projects, tags, folders) |
| Destructive | `drop`, `delete` |

Example prompts: "What's flagged and due this week?", "Add these five tasks to the Groceries project", "Move everything tagged `waiting` in my inbox to the Follow-ups project", "Put the Renovation project on hold".

## Tool contract

Conventions:

- Ids are OmniFocus `primaryKey` strings.
- Dates are ISO 8601 strings: `2026-11-18`, `2026-11-18T08:00:00`, or with an offset (`2026-11-18T08:00:00Z`, `...+01:00`). No offset means local time. A date-only value means 00:00 local time. Returned dates are local time without an offset.
- In patches, an omitted field means unchanged and an explicit `null` clears (dates, note).
- Batch tools apply items in order and return one result per item (`ok` or `error`). One failing item never aborts the batch.
- Invalid input is rejected before any script runs.

Getters are per type (`get_task`, `get_project`, `get_tag`, `get_folder`) rather than one `get`, so each has a precise output schema instead of a union.

### Read (read-only)

- `list_tasks`: optional filters, combined with AND:
  - `inbox` (bool; includes nested inbox tasks; can't be combined with `project`)
  - `project`, `tag` (id or name)
  - `available` (bool: not blocked, deferred, completed or dropped), `flagged`, `completed` (bool)
  - `due_before`, `defer_before` (dates)
  - `search` (substring over name and note)
  - `limit` (default 50; `null` means all)

  Each task has id, name, note, flagged, project, parent, tags, defer/due/planned dates, completed and status. It also has `estimatedMinutes` and `repetition` (`{rule, method}`: RRULE string and repetition method) when they are set.
- `list_projects`: `status` filter (`Active`, `OnHold`, `Done`, `Dropped`), optional `folder`, `search`, `limit`.
- `list_tags`: includes status and `remainingTasks`.
- `list_folders`: includes parent and status.
- `list_perspectives`: `{perspectives: [{id|null, name, builtin}]}`.
- `get_task`, `get_project`, `get_tag`, `get_folder`: `{id}`; error if not found.

### Task writes

- `add_tasks`: `{tasks: [{name, note?, flagged?, defer?, due?, planned?, project? | parent? (default inbox), tags?: [id|name]}]}`. Returns `{results: [{id, ok, error?, task?}]}`, one per task in order, with the created task records. Tags are given by id or by a name that matches exactly one tag.
- `edit_tasks`: `{patches: [{id, name?, note?, flagged?, defer?, due?, planned?, complete? | uncomplete?, move?: {project: id} | {inbox: true} | {parent: id}, tags?: {add?: [..], remove?: [..]}}]}`.
  - `complete` and `uncomplete` are mutually exclusive, and `move` takes exactly one target.
  - `null` clears `note` and the three dates. `name` cannot be empty, and a patch must change something.
  - Returns `{results: [{id, ok, error?, task?}]}` in input order, with the updated task records.
  - An item that fails validation (bad date, empty name, contradictory move, unknown or ambiguous tag) or fails in OmniFocus (unknown id) gets `ok: false` and leaves the others untouched.
  - Edits within one item are not atomic. If a later field throws, the earlier field changes stay applied and the item reports `ok: false`. Re-read the task before retrying.

A project id is not a task id. `get_task`, `edit_tasks`, `add_tasks` parents and `drop`/`delete` with kind `task` reject it with "is a project, use kind project".

### Structure writes

Each tool takes `{items: [...]}` and returns `{results: [{id, kind, ok, error?, name?, status?, parent?}]}` in input order. An invalid or refused item gets `ok: false` and leaves the others untouched.

- `add`: items `{kind: project|tag|folder, name, parent?, status?, sequential? (project)}`. The parent is a folder for projects and folders, or a tag for tags. Omit it to create at the root.
- `edit`: items `{kind, id, name?, status?, sequential? (project), move?: {parent: id | null}, reorder?: {before|after: id}}`.
  - `move` and `reorder` are mutually exclusive. A reorder adopts the sibling's parent.
  - Moving an object into itself or a descendant is refused.
  - Project status: `Active`, `OnHold`, `Done`.
  - Tag status: `Active`, `OnHold`. `Active` re-activates a dropped tag.
  - Folders: rename, move and reorder only.

### Destructive

Items are `{kind: task|project|tag|folder, id}`, with the same result shape as structure writes. The safety checks run inside OmniFocus, because only it can count remaining tasks, and a refusal appears as that item's `error`.

- `drop`: you can undo it through this server only for projects and tags, by editing their status back to `Active`. A dropped folder, which also drops its projects, or a dropped task cannot be restored through these tools. Dropping a project drops its tasks. A tag can be dropped only when neither it nor any descendant tag has remaining tasks.
- `delete`: irreversible.
  - Tags and folders are deleted only when already dropped and empty, so delete children first.
  - A project is deleted only when it has no remaining (incomplete) tasks. A dropped project can always be deleted.
  - A task is deleted only when it has no remaining subtasks. Otherwise, drop it instead.

## Troubleshooting

- **"OmniFocus is not running"**: start OmniFocus and retry.
- **"not authorized to control OmniFocus"**: allow the host app under System Settings > Privacy & Security > Automation. If no prompt ever appeared, run `tccutil reset AppleEvents` (this resets Automation permission for every app) and try again.
- **A write timed out**: reads time out after 30 s and writes after 120 s. A write that times out may still finish, or partly apply, in OmniFocus. List the affected objects before retrying.

## Development

```sh
make ready   # gofmt check, go vet, unit tests, build
make live    # integration tests against the real OmniFocus
```

Unit tests fake the script runner and never touch OmniFocus. `make live` needs OmniFocus running with Automation permission. It reads your database and creates, edits and deletes its own objects: tasks in an "MCP Sandbox" project (the project itself stays), an "MCP Sandbox Folder" and `mcp-test-*` tags. Run it on a test database if you'd rather not have those appear in your sync.

Layout:

- `main.go`: stdio entry point.
- `internal/omni`: the bridge to OmniFocus, plus the embedded Omni Automation scripts in `js/`. Every call goes through one function. It passes arguments as a JSON literal that the script parses, so user values are never spliced into code. The script reaches `osascript` on stdin rather than argv.
- `internal/server`: tool registration and input/output types.

## License

[MIT](LICENSE)
