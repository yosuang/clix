# clix MVP Product Spec v0.1

## Positioning

`clix` is a local tool layer for AI coding/dev agents. It turns real project operations, such as internal APIs, scripts, and product CLIs, into discoverable, typed, approvable, and auditable actions.

Humans are not the primary caller. Humans use the CLI to define tools, approve sensitive operations, inspect runs, and debug behavior. The primary caller is an AI agent.

## Core Principles

- CLI-only for MVP. MCP is not included yet.
- Default output is concise semantic text. Machine callers use `--json` and optional `--jq` to select the fields they need.
- There is no human interactive mode.
- The core object is a semantic tool/action, not a workflow.
- `clix` is not an agent and does not call AI models.
- Adapters are built into `clix`. MVP does not include an external adapter protocol.
- Every tool must declare `effect: read` or `effect: write`.
- `read` tools execute immediately.
- `write` tools create a `pending_approval` run and only execute after `approve`.
- No dry-run in MVP. A fake dry-run is more dangerous than no dry-run.
- All runs are recorded in a user-global SQLite database.

## Technical Architecture

MVP is a single Go CLI binary.

```text
CLI
  -> output layer
  -> manifest loader and validator
  -> tool registry
  -> run service
      -> SQLite run store
      -> input schema validator
      -> built-in adapter registry
          -> http
```

The CLI layer parses arguments and writes output. It does not execute tools directly.

The run service owns validation, approval, state transitions, fingerprint checks, and adapter execution. This keeps approval behavior independent from the command that triggered it.

The adapter registry is internal. Adding a new adapter, such as `script` or `db.postgres`, requires a `clix` release. Extensibility in MVP comes from adding more tools to the manifest, not from installing external adapter processes.

## User Storage Layout

```text
~/.config/clix/
  manifest.yaml

~/.local/share/clix/
  clix.db
```

`~` resolves to the current user's home directory.

The manifest is user configuration. The database is user state. Neither file is project-local in MVP.

`clix` creates the parent directories when needed.

## Manifest

The manifest lives at `~/.config/clix/manifest.yaml`.

It uses a strict, statically analyzable YAML subset. At runtime, `clix` parses it into canonical JSON for validation, fingerprinting, and execution.

Minimal example:

```yaml
version: 1

tools:
  weekly.get_records:
    description: Get work records for a given week.
    adapter: http
    effect: read
    input_schema:
      type: object
      additionalProperties: false
      required: [week]
      properties:
        week:
          type: string
    output_schema:
      type: object
    http:
      method: GET
      url: "https://example.com/api/records?week=${input.week}"
```

Tool names must match:

```text
^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$
```

This means:

- At least two dot-separated segments, such as `weekly.get_records`.
- Lowercase letters, digits, and underscores only.
- No spaces, dashes, slashes, or uppercase letters.

Tool names describe the semantic action. They should not include the adapter name unless the adapter is part of the user-facing concept.

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

## Adapters

MVP supports one built-in adapter:

- `http`

Future built-in adapters may include:

- `script`
- `db.postgres`

All adapters share the same top-level contract:

```text
input: JSON object
output: JSON
```

### HTTP Adapter

The `http` adapter supports simple variable substitution only:

```text
${input.x}
${secrets.X}
```

It does not support conditionals, loops, functions, JavaScript expressions, complex JSONPath, or default-value expressions.

HTTP responses must be JSON. Non-JSON responses return `INVALID_ADAPTER_OUTPUT`.

Secrets are read only from environment variables. MVP does not include a secret store.

Example:

```yaml
tools:
  weekly.submit_report:
    description: Submit a weekly report.
    adapter: http
    effect: write
    secrets:
      - WORK_API_TOKEN
    input_schema:
      type: object
      additionalProperties: false
      required: [week, content]
      properties:
        week:
          type: string
        content:
          type: string
    output_schema:
      type: object
    http:
      method: POST
      url: "https://example.com/api/reports"
      headers:
        Authorization: "Bearer ${secrets.WORK_API_TOKEN}"
      json_body:
        week: "${input.week}"
        content: "${input.content}"
```

## Schema

- `input_schema` is required and enforced.
- `input_schema.type` must be `object`.
- `input_schema` uses a JSON Schema subset.
- `output_schema` is required but not enforced in MVP.

MVP supports exactly these JSON Schema keywords:

- `type`
- `properties`
- `required`
- `items`
- `enum`
- `minimum`
- `maximum`
- `minLength`
- `maxLength`
- `additionalProperties`

MVP does not commit to advanced JSON Schema features such as:

- `$ref`
- `oneOf`
- `anyOf`
- `allOf`
- `if` / `then` / `else`
- `patternProperties`
- strict `format` semantics
- custom keywords

## Runs

Runs are stored in a user-global SQLite database:

```text
~/.local/share/clix/clix.db
```

The database may contain sensitive input. It must not be committed or treated as project state.

Minimal run fields:

```text
id
tool_name
effect
tool_fingerprint
input_json
status
requested_at
approved_at
started_at
finished_at
exit_code
error_code
error_message
```

All actions create run records, including `read` actions.

Read state transitions:

```text
created -> running -> succeeded
created -> running -> failed
```

Write state transitions:

```text
created -> pending_approval
pending_approval -> running -> succeeded
pending_approval -> running -> failed
pending_approval -> rejected
```

`approve` must use a transaction to prevent duplicate execution. A pending run can only move to `running` once.

Failed runs are terminal. Retrying requires creating a new run.

Pending write runs store a tool fingerprint. On `approve`, if the current tool definition fingerprint differs from the stored fingerprint, execution is rejected with `MANIFEST_CHANGED`.

The fingerprint is computed per tool definition, not per manifest file. It includes the normalized tool definition, including adapter name and adapter config, excluding resolved secret values.

## Commands

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

## Execution Rules

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
