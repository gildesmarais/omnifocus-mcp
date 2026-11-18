# AGENTS.md

Go MCP server for OmniFocus. The tool contract lives in `README.md` (single owner); implement it, do not restate it.

- Gate: `mise exec -- make lint check build` must exit 0.
- Mutation testing runs via `make mutate` (`DIFF=<base>` for changed code) per tranche; opt-in, not part of the gate.
- Live test (`make live`, build tag `live`) talks to the running OmniFocus. Run it only on request or when changing `internal/omni` JS. T0 live tests are read-only; any later live test that writes must create and clean up its own uniquely named objects.
- Layout: `main.go`, `internal/omni` (bridge + embedded `js/*.js`), `internal/server` (tools, input/output types). Fake the `omni.Runner` in tests; never shell out in unit tests.
- Never interpolate user values into JS. Arguments reach scripts only via `Bridge.call` (JSON literal, parsed in the script).
- Each script in `internal/omni/js` is a function expression `(function (args) { ... })` returning JSON-serialisable data.
- Patch types validate at construction; destructive operations stay in their own tools (`drop`, `delete`) so clients can gate them by name.
