# clix MVP Product Design v0.1

## Positioning

`clix` is a local tool layer for AI coding/dev agents. It turns real project operations, such as internal APIs, scripts, and product CLIs, into discoverable, typed, approvable, and auditable actions.

Humans are not the primary caller. Humans use the CLI to define tools, approve sensitive operations, inspect runs, and debug behavior. The primary caller is an AI agent.

## Product Principles

- CLI-only for MVP. MCP is not included yet.
- Default output is concise semantic text. Machine callers use `--json` and optional `--jq` to select the fields they need.
- There is no human interactive mode.
- The core object is a semantic tool/action, not a workflow.
- `clix` is not an agent and does not call AI models.
- Every tool must declare `effect: read` or `effect: write`.
- `read` tools execute immediately.
- `write` tools create a `pending_approval` run and only execute after `approve`.
- No dry-run in MVP. A fake dry-run is more dangerous than no dry-run.
- All runs are recorded in a user-global SQLite database.

## User-Facing Model

A tool represents a semantic action. Tool names should describe what the caller wants to do, not the adapter used to do it.

Good:

```text
github.repo.get
github.repo.search
weekly.submit_report
```

Avoid:

```text
http.github.repo.get
http.weekly.submit_report
```

The MVP gives agents three things they need to safely use project operations:

- Discover tools with names, descriptions, effects, adapters, and schemas.
- Run read actions immediately and receive JSON output.
- Request write actions through an approval gate before execution.

## Command Surface

MVP command surface:

```bash
clix check
clix tools list
clix tools get <tool_name>
clix run <tool_name> --input '<json>'
clix approve <run_id>
clix reject <run_id>
clix runs list [--status <status>]
clix runs get <run_id>
```

All commands accept output flags:

```text
[--json <fields>] [--jq <expr>]
```

All commands write their primary result to stdout and diagnostics to stderr.

Default output is concise semantic text. It is meant for a human to scan and for an agent to keep in context without carrying unused fields.

Structured output uses `--json`:

```bash
clix tools list --json name,effect,adapter,description
clix runs get run_xxx --json id,tool_name,status,error_code,error_message
```

`--json` accepts a comma-separated list of top-level fields. It returns only those fields. For list commands, every item is projected to the requested fields.

`--jq` filters the JSON result before stdout:

```bash
clix tools list --json name,effect,adapter --jq '.[] | select(.effect == "write")'
```

`--jq` implies JSON output. If `--jq` is used without `--json`, the jq expression receives the full command result. If both flags are present, field projection runs before the jq expression.

Success in JSON mode returns the selected command result directly:

```json
[
  {
    "name": "weekly.submit_report",
    "effect": "write",
    "adapter": "http"
  }
]
```

Failures in JSON mode return a stable error object:

```json
{
  "ok": false,
  "code": "VALIDATION_ERROR",
  "message": "..."
}
```

Default failure output includes the same error code in text form.

## Execution Behavior

`read` tool:

```text
clix run <tool_name> --input '<json>'
```

creates a run, executes immediately, and returns the adapter output.

In JSON mode, callers can select run metadata and output:

```bash
clix run weekly.get_records --input '{"week":"current"}' --json id,status,output
```

`write` tool:

```text
clix run <tool_name> --input '<json>'
```

creates a `pending_approval` run and does not execute the adapter.

In JSON mode, callers normally request only approval metadata:

```bash
clix run weekly.submit_report --input '{"week":"current","content":"..."}' --json id,status,tool_name
```

Then:

```text
clix approve <run_id>
```

approves and immediately executes the pending run.

`reject` only works for `pending_approval` runs and moves the run to `rejected`.

Adapter output is returned by the command that executed the adapter. It is not persisted. `runs get` returns run metadata, status, timing fields, and error fields.

## Explicitly Out Of Scope

- MCP server
- HTTP server
- workflow engine
- SDK
- browser adapter
- computer use
- SQL adapter in MVP
- external adapter protocol
- dry-run
- rich pretty output
- interactive prompts
- project-local manifest discovery
- secret store
- output persistence
- retry
- namespace policy
- AI adapter

## MVP Acceptance Scenario

Use a weekly report workflow to validate the product:

```bash
clix tools list --json name,effect,adapter,description
clix tools get weekly.get_records --json name,effect,input_schema,output_schema
clix run weekly.get_records --input '{"week":"current"}' --json id,status,output
clix run weekly.submit_report --input '{"week":"current","content":"..."}' --json id,status,tool_name
clix approve run_xxx --json id,status,output
clix runs get run_xxx --json id,tool_name,status,error_code,error_message
```

Success criteria:

- An agent can understand tools from `tools list` and `tools get`.
- Tool names describe semantic actions, not adapter mechanics.
- A `read` action executes immediately and returns JSON in JSON mode.
- A `write` action always creates a pending run first.
- `approve` executes a write run exactly once.
- A pending write run is rejected if its tool definition changed.
- `--json` and `--jq` let callers remove fields they do not need.
- All errors have stable JSON codes.
- The manifest is loaded from `~/.config/clix/manifest.yaml`.
- Runs are stored in `~/.local/share/clix/clix.db`.
