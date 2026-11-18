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
- Dates are ISO 8601 strings: `2026-11-18`, `2026-11-18T08:00:00`, or with an offset (`2026-11-18T08:00:00Z`, `...+01:00`). No offset means local time. A date-only value means 00:00 local time when written; as a filter bound it covers its whole day (see `list_tasks`). Returned dates are local time without an offset.
- In patches, an omitted field means unchanged and an explicit `null` clears (dates, note, `estimatedMinutes`, `repetition`).
- `null` on a field that cannot be cleared is rejected by the schema and fails the whole call before any script runs; only clearable fields accept `null` per item. The same holds for a value outside a field's schema: a repetition `scheduleType` or `anchor`, review `unit` or project `type` not in the listed names, a negative `estimatedMinutes` or `steps` below 1. `limit: null` (all) is the one `null` the list tools accept.
- Batch tools apply items in order and return one result per item (`ok` or `error`). One failing item never aborts the batch.
- Invalid input is rejected before any script runs.

Getters are per type (`get_task`, `get_project`, `get_tag`, `get_folder`) rather than one `get`, so each has a precise output schema instead of a union.

### Read (read-only)

- `list_tasks`: optional filters, combined with AND:
  - `inbox` (bool; includes nested inbox tasks; can't be combined with `project`)
  - `project`, `tag` (id or name)
  - `available` (bool: not blocked, deferred, completed or dropped), `flagged`, `completed` (bool)
  - `untagged` (no tags), `repeating` (has a repetition rule), `dropped` (bools: `true` returns only those, `false` excludes them)
  - Range filters (dates), each on one date: `due_before`, `due_after`, `defer_before`, `defer_after`, `planned_before`, `planned_after`, `completed_before`, `completed_after` (the completion date), `modified_before`, `modified_after`, `added_before`, `added_after`. `_before` means on or before and `_after` on or after. A date-only value covers its whole local day: as a `_before` bound it ends at 23:59:59 that day, as an `_after` bound it starts at 00:00:00. A task without that date never matches a bound on it. `review_due_before` on `list_projects` follows the same rule.
  - `search` (substring over name and note)
  - `limit` (default 50; `null` means all)

  `list_tasks` hides nothing by default: completed and dropped tasks are returned too, so the `completed_*` bounds and `dropped: true` find them without any extra switch. Contradictory filters are rejected before any script runs: an `_after` later than its `_before`, `completed: false` with a `completed_*` bound, `dropped: true` with `completed: true` or a `completed_*` bound, and `available: true` with `completed: true`, a `completed_*` bound or `dropped: true`.

  Each task has id, name, note, flagged, project, parent, tags, defer/due/planned dates, completed and status. It also has:
  - `hasChildren`, `sequential` (subtasks run in order) and `completedByChildren` (completes with its last subtask), always present.
  - `added` and `modified` dates, always present.
  - `completedAt` and `dropDate` dates, when set.
  - `estimatedMinutes` and `repetition`, when set. `repetition` is `{rule, scheduleType, anchor, catchUp}`: an RRULE string, `Regularly` (fixed schedule) or `FromCompletion` (the next occurrence counts from completion), the date the rule moves (`DeferDate`, `DueDate` or `PlannedDate`) and whether missed occurrences are skipped.
- `list_projects`: `status` filter (`Active`, `OnHold`, `Done`, `Dropped`), optional `folder`, `search`, `review_due_before` (a date; projects whose `nextReviewDate` is on or before it, a date-only value covering its whole day), `limit`.

  Each project (also from `get_project`) has id, name, note, status, flagged, tags, defer/due/planned dates, `available`, `blocked` and folder. It also has:
  - `type`, always present: `Parallel`, `Sequential` (tasks run in order) or `SingleActions` (a list of unrelated actions). OmniFocus can hold both the sequential and the single-actions flag at once. `SingleActions` wins on read.
  - `completedByChildren` (completes with its last task), always present.
  - `estimatedMinutes` and `repetition` (same shape as on tasks), when set.
  - `reviewInterval` (`{steps, unit}`), `nextReviewDate` and `lastReviewDate`, when set.

  The review unit is `days`, `weeks`, `months` or `years` (`hours` can appear on read). Combine `review_due_before` with `status: Active` to get the review queue, since completed and dropped projects keep their review dates.
- `list_tags`: includes status and `remainingTasks`.
- `list_folders`: includes parent and status.
- `list_perspectives`: `{perspectives: [{id|null, name, builtin}]}`.
- `get_task`, `get_project`, `get_tag`, `get_folder`: `{id}`; error if not found.

### Task writes

- `add_tasks`: `{tasks: [{name, note?, flagged?, defer?, due?, planned?, project? | parent? (default inbox), tags?: [id|name], estimatedMinutes?, repetition?, sequential?, completedByChildren?}]}`. `estimatedMinutes` is an integer of at least 0. `repetition` is the object returned on read, `{rule, scheduleType, anchor, catchUp}`, with all four fields required; `scheduleType` is `Regularly` or `FromCompletion`, and a rule anchored on a date the task lacks is accepted. Returns `{results: [{id, ok, error?, task?}]}`, one per task in order, with the created task records. Tags are given by id or by a name that matches exactly one tag.
- `edit_tasks`: `{patches: [{id, name?, note?, flagged?, defer?, due?, planned?, complete? | uncomplete?, move?: {project: id} | {inbox: true} | {parent: id}, tags?: {add?: [..], remove?: [..]}, estimatedMinutes?, repetition?, sequential?, completedByChildren?, restore?}]}`.
  - `complete` and `uncomplete` are mutually exclusive, and `move` takes exactly one target.
  - `restore: true` un-drops a dropped task (it becomes available again and loses its `dropDate`). It cannot be combined with `complete`.
  - `null` clears `note`, the three dates, `estimatedMinutes` and `repetition`. `name` cannot be empty, and a patch must change something.
  - `estimatedMinutes` and `repetition` take the same values as in `add_tasks`.
  - Returns `{results: [{id, ok, error?, task?}]}` in input order, with the updated task records.
  - An item that fails validation (bad date, empty name, negative estimate, incomplete repetition, contradictory move or restore, unknown or ambiguous tag) or fails in OmniFocus (unknown id) gets `ok: false` and leaves the others untouched.
  - Edits within one item are not atomic. If a later field throws, the earlier field changes stay applied and the item reports `ok: false`. Re-read the task before retrying.

A project id is not a task id. `get_task`, `edit_tasks`, `add_tasks` parents and `drop`/`delete` with kind `task` reject it with "is a project, use kind project".

### Structure writes

Each tool takes `{items: [...]}` and returns `{results: [{id, kind, ok, error?, name?, status?, parent?}]}` in input order. An invalid or refused item gets `ok: false` and leaves the others untouched.

- `add`: items `{kind: project|tag|folder, name, parent?, status?, type?, defer?, due?, planned?, flagged?, note?, tags?: [id|name], estimatedMinutes?, repetition?, completedByChildren?}`. The parent is a folder for projects and folders, or a tag for tags. Omit it to create at the root.
  - `type`, `defer`, `due`, `planned`, `flagged`, `note`, `tags`, `estimatedMinutes`, `repetition` and `completedByChildren` are for projects only and are rejected on tags and folders. They take the same values as in `add_tasks`. `type` is `Parallel` (the default), `Sequential` or `SingleActions`.
  - The `add` schema never allows `null`: adding has nothing to clear.
- `edit`: items `{kind, id, name?, status?, move?: {parent: id | null}, reorder?: {before|after: id}}` plus, for projects, `type?`, `note?`, `flagged?`, `defer?`, `due?`, `planned?`, `tags?: {add?: [..], remove?: [..]}`, `estimatedMinutes?`, `repetition?` and `completedByChildren?`.
  - `move` and `reorder` are mutually exclusive. A reorder adopts the sibling's parent.
  - Moving an object into itself or a descendant is refused.
  - Project status: `Active`, `OnHold`, `Done`.
  - Tag status: `Active`, `OnHold`. `Active` re-activates a dropped tag.
  - Folder status: `Active` only. It restores a dropped folder, and its projects come back active. Any other folder status is refused. Folders can also be renamed, moved and reordered.
  - Project fields (rejected on tags and folders) follow `edit_tasks`: an omitted field is unchanged, and `null` clears `note`, the three dates, `estimatedMinutes` and `repetition`. `tags` takes `{add, remove}` by id or unambiguous name, and a tag cannot be in both lists. `type` always sets the project to exactly that type. A patch must change something.
  - Project review (rejected on tags and folders):
    - `reviewInterval: {steps, unit}` with `steps` an integer of at least 1 and `unit` one of `days`, `weeks`, `months`, `years`. It cannot be cleared. Changing it does not move `nextReviewDate`.
    - `nextReviewDate` (a date). OmniFocus truncates it to local midnight, so `2026-12-01T13:45:00` is stored as `2026-12-01T00:00:00`.
    - `reviewed: true` marks the project reviewed now. OmniFocus sets `lastReviewDate` to now and `nextReviewDate` to the last review plus the interval, at local midnight. It cannot be combined with `nextReviewDate`. With `reviewInterval` in the same item, the new interval applies.

### Destructive

Items are `{kind: task|project|tag|folder, id}`, with the same result shape as structure writes. The safety checks run inside OmniFocus, because only it can count remaining tasks, and a refusal appears as that item's `error`.

- `drop`: you can undo it through this server for projects, tags and folders, by editing their status back to `Active`, and for tasks with `edit_tasks` `restore: true`. Dropping a project drops its tasks, and dropping a folder drops its projects. A tag can be dropped only when neither it nor any descendant tag has remaining tasks.
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
make mutate  # opt-in mutation testing (gremlins, all mutators via .gremlins.yaml); PKG=./internal/omni narrows it, DIFF=main mutates only code changed since main
```

Unit tests fake the script runner and never touch OmniFocus. `make live` needs OmniFocus running with Automation permission. It reads your database and creates, edits and deletes its own objects: tasks in an "MCP Sandbox" project (the project itself stays), an "MCP Sandbox Folder", `mcp-test-*` folders, tags and projects. Run it on a test database if you'd rather not have those appear in your sync.

Layout:

- `main.go`: stdio entry point.
- `internal/omni`: the bridge to OmniFocus, plus the embedded Omni Automation scripts in `js/`. Every call goes through one function. It passes arguments as a JSON literal that the script parses, so user values are never spliced into code. The script reaches `osascript` on stdin rather than argv.
- `internal/server`: tool registration and input/output types.

## License

[MIT](LICENSE)
