# clix MVP Technical Design v0.2

## Design Center

`clix` is an agent-facing Unix CLI. The stable technical contract is the command-line protocol: stdin, stdout, stderr, exit codes, structured projection, error shape, and run state transitions.

The internal implementation stays small and replaceable:

```text
CLI protocol layer
  -> command handlers
  -> output projector and jq filter
  -> manifest registry
  -> run service
      -> SQLite run store
      -> input validator
      -> built-in adapter registry
          -> http
```

The CLI protocol layer owns how `clix` speaks to agents and humans. The run service owns validation, approval, state transitions, fingerprint checks, and adapter execution. Adapters only execute external operations.

This keeps the MVP focused while leaving room for future MCP, command adapters, or batch input without replacing the CLI contract.

## Standard Streams

All commands follow standard Unix stream behavior:

- `stdout` contains only the primary result.
- `stderr` contains diagnostics and error text.
- Exit code `0` means success.
- Any non-zero exit code means failure.

Default output is concise semantic text. It is meant to be readable by humans and cheap for agents to keep in context. It is not a rich UI and does not include progress bars, spinners, colors, or long explanations.

JSON mode still uses exit codes. On failure, the JSON error object is written to `stdout` and the process exits non-zero.

## Command Surface

MVP command surface:

```bash
clix check
clix tools list
clix tools get <tool_name>
clix run <tool_name> [--input '<json>']
clix approve <run_id>
clix reject <run_id>
clix runs list [--status <status>]
clix runs get <run_id>
```

All commands accept output flags:

```text
--json <fields>
--jq <expr>
```

`--dry-run` is not supported. Most real operations cannot be simulated safely, and a fake dry-run is more dangerous than no dry-run.

## Input Protocol

`clix run <tool_name>` accepts one JSON object as input.

Input can come from `--input`:

```bash
clix run weekly.get_records --input '{"week":"current"}'
```

If `--input` is absent, `clix` reads one JSON object from `stdin`:

```bash
make-input | clix run weekly.get_records
```

Rules:

- `--input` has explicit priority.
- If `--input` is present and piped/file stdin contains non-whitespace bytes, the command fails with `USAGE_ERROR`.
- If `--input` is absent, stdin must contain exactly one JSON object.
- Empty input fails with `VALIDATION_ERROR`.
- Multiple JSON values fail with `VALIDATION_ERROR`.
- A top-level array, string, number, boolean, or null fails with `VALIDATION_ERROR`.

After parsing, input is canonicalized before schema validation, run storage, fingerprint-sensitive approval behavior, and adapter execution.

## Output Protocol

Default text output is terse.

Example `tools list` output:

```text
weekly.get_records read http - Get work records for a given week.
weekly.submit_report write http - Submit a weekly report.
```

Example run output:

```text
run_abcd1234 succeeded weekly.get_records
```

For structured output, callers use field projection:

```bash
clix runs get run_abcd1234 --json id,status,error_code,error_message
clix tools list --json name,effect,adapter
```

Rules:

- `--json <fields>` enables JSON output.
- `<fields>` is a comma-separated list of top-level fields.
- Object results are projected to those fields.
- List results project every item to those fields.
- Missing projected fields are omitted.
- Nested projection is not supported.

`--jq <expr>` filters JSON output:

```bash
clix tools list --json name,effect,adapter --jq '.[] | select(.effect == "write")'
```

Rules:

- `--jq` implies JSON output.
- If `--jq` is used without `--json`, the jq expression receives the full command result.
- If both are present, field projection runs before jq filtering.
- jq output is written directly as JSON.

Successful JSON output is the selected command result directly. It is not wrapped in an `ok: true` envelope.

## Error Protocol

Default-mode failures write one short line to `stderr`:

```text
VALIDATION_ERROR: input.week is required
```

JSON-mode failures write a stable object to `stdout`:

```json
{
  "ok": false,
  "code": "VALIDATION_ERROR",
  "message": "input.week is required"
}
```

Rules:

- `code` is the stable API for branching.
- `message` is the only human-readable explanation.
- `message` must be short, concrete, and local to the failure.
- No `hint`, `details`, `suggestion`, stack trace, or nested diagnostics are included in MVP.

Stable MVP error codes:

```text
ADAPTER_ERROR
APPROVAL_ERROR
INTERNAL_ERROR
INVALID_ADAPTER_OUTPUT
JQ_ERROR
MANIFEST_CHANGED
MANIFEST_ERROR
MISSING_SECRET
RUN_NOT_FOUND
STORAGE_ERROR
TOOL_NOT_FOUND
USAGE_ERROR
VALIDATION_ERROR
```

## Execution And Approval

All actions create run records, including reads.

`read` tool behavior:

```text
created -> running -> succeeded
created -> running -> failed
```

`write` tool behavior:

```text
created -> pending_approval
pending_approval -> running -> succeeded
pending_approval -> running -> failed
pending_approval -> rejected
```

Rules:

- `read` tools validate input and execute immediately.
- `write` tools validate input and stop at `pending_approval`.
- `approve <run_id>` only works for `pending_approval` runs.
- `approve` uses a transaction to claim the run before adapter execution.
- A pending run can move to `running` at most once.
- `reject <run_id>` only works for `pending_approval` runs.
- `succeeded`, `failed`, and `rejected` are terminal.
- Retrying requires creating a new run.

Pending write runs do not expose a request preview. Default output is only run metadata:

```text
run_abcd1234 pending_approval weekly.submit_report
```

Agents that need structured metadata request it explicitly:

```bash
clix run weekly.submit_report --json id,status,tool_name,effect
```

Approval safety comes from the stored input, the tool definition, and the tool fingerprint. On `approve`, if the current tool definition fingerprint differs from the stored fingerprint, execution is rejected with `MANIFEST_CHANGED` and the adapter is not executed.

## User Storage Layout

```text
~/.config/clix/
  manifest.yaml

~/.local/share/clix/
  clix.db
```

The manifest is user configuration. The database is user state. Neither file is project-local in MVP.

`~` resolves to the current user's home directory. `clix` creates parent directories when needed.

The database may contain sensitive input. It must not be committed or treated as project state.

## Manifest

The manifest lives at:

```text
~/.config/clix/manifest.yaml
```

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

Tool names describe semantic actions. They should not include the adapter name unless the adapter is part of the user-facing concept.

Every tool must declare:

- `description`
- `adapter`
- `effect`
- `input_schema`
- `output_schema`

`effect` must be either `read` or `write`.

MVP only supports the `http` adapter.

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

MVP does not support advanced JSON Schema features such as:

- `$ref`
- `oneOf`
- `anyOf`
- `allOf`
- `if` / `then` / `else`
- `patternProperties`
- strict `format` semantics
- custom keywords

## HTTP Adapter

MVP supports one built-in adapter:

- `http`

Adapters are built into `clix`. MVP does not include an external adapter protocol.

All adapters share the same top-level contract:

```text
input: JSON object
output: JSON
```

The `http` adapter supports simple variable substitution only:

```text
${input.x}
${secrets.X}
```

It does not support conditionals, loops, functions, JavaScript expressions, complex JSONPath, or default-value expressions.

HTTP responses must be JSON. Non-JSON responses return `INVALID_ADAPTER_OUTPUT`.

Secrets are read only from environment variables. MVP does not include a secret store. A secret must be declared by name before it can be referenced.

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

## Run Store

Runs are stored in:

```text
~/.local/share/clix/clix.db
```

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

Storage rules:

- `input_json` is stored so approved write runs can execute later.
- Public command results expose the stored input as `input`, not `input_json`.
- Adapter output is returned by the command that executes the adapter.
- Adapter output is not persisted in MVP.
- `runs get` default text output does not show input.
- `runs get --json input` may expose stored input explicitly.

The fingerprint is computed per tool definition, not per manifest file. It includes the normalized tool definition, including adapter name and adapter config, excluding resolved secret values.

## Acceptance Tests

Implementation tests should focus on CLI protocol behavior, not only service internals.

Required paths:

- `stdout` and `stderr` separation: successful primary results only go to `stdout`; default errors only go to `stderr`.
- stdin input: `clix run` reads one JSON object from stdin when `--input` is absent.
- input conflict: `--input` plus non-empty piped/file stdin fails with `USAGE_ERROR`.
- JSON projection: `--json id,status` returns only those top-level fields.
- jq filtering: jq-only receives the full command result; json-plus-jq projects before jq.
- JSON failure: failures in JSON mode return exactly `ok`, `code`, and `message`.
- read execution: read tools execute immediately and return adapter output.
- write approval: write runs stop at `pending_approval`; approve executes exactly once.
- manifest changed: changed tool fingerprints reject approval with `MANIFEST_CHANGED` before adapter execution.
- no dry-run: the command surface does not include `--dry-run`.

MVP success criteria:

- An agent can discover tools with `tools list` and `tools get`.
- An agent can request only the fields it needs with `--json`.
- An agent can compose input through stdin or pass it explicitly with `--input`.
- Errors are short, stable, and branchable by code.
- Primary results are safe to pipe because diagnostics do not pollute `stdout`.
- Write actions cannot execute before approval.
