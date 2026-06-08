# clix MVP Technical Design v0.4

## Design Center

`clix` is an agent-facing Unix CLI. The stable technical contract is the command-line protocol: stdin, stdout, stderr, exit codes, structured projection, error shape, and run state transitions.

The internal implementation stays small and replaceable:

```text
CLI protocol layer
  -> command handlers
  -> output projector
  -> tool catalog
      -> scan ~/.config/clix/tools/
      -> parse tool files
      -> validate and normalize
      -> build name index
  -> run service
      -> SQLite run store
      -> loaded tool input validator
      -> built-in adapter registry
          -> http
```

The CLI protocol layer owns how `clix` speaks to agents and humans. The tool catalog owns discovery, parsing, validation, normalization, compiled input validators, and lookup of tool definitions. The run service owns runtime input validation orchestration, approval, state transitions, fingerprint checks, and adapter execution. Adapters only execute external operations.

This keeps the MVP focused while leaving room for future MCP, command adapters, or batch input without replacing the CLI contract.

## Implementation Stack

`clix` is implemented in Go. The implementation uses mainstream, boring dependencies and keeps the stable CLI protocol in `clix` code instead of delegating it to framework defaults.

Core dependencies:

| Area | Dependency | Reason |
|---|---|---|
| CLI and flags | `github.com/spf13/cobra` | De facto Go CLI framework. It provides nested commands, POSIX-style flags through pflag, help, completion, and command routing. |
| YAML parsing | `github.com/goccy/go-yaml` | Stable YAML parser with parser/AST support for rejecting unsupported YAML syntax. |
| JSON Schema validation | `github.com/santhosh-tekuri/jsonschema/v6` | Stable validator with modern JSON Schema support. `clix` still rejects unsupported MVP keywords before compiling schemas. |
| SQLite | `database/sql` + `github.com/mattn/go-sqlite3` | Most established Go SQLite driver. The tradeoff is CGO. |
| HTTP | Go standard library `net/http` | The MVP HTTP adapter does not need a higher-level client. |
| Run IDs | `github.com/google/uuid` | Stable UUID package. Runs use UUIDv7 IDs exposed as `run_<uuid>`. |

Dependency rules:

- Add dependencies with explicit stable versions, such as `go get <module>@vX.Y.Z`.
- Do not use prerelease versions such as alpha, beta, or release candidate dependencies unless a design note explicitly approves the exception.
- Pin exact versions through `go.mod` and `go.sum`.
- Do not use Viper in MVP. Configuration sources must stay explicit.
- If CGO-free distribution becomes a hard release requirement, replace `github.com/mattn/go-sqlite3` with `modernc.org/sqlite` behind the same store interface.

## Go Package Layout

The executable entry point lives in `cmd/clix`. Supporting implementation lives under `internal` because the MVP does not expose a Go SDK or reusable command package.

```text
cmd/clix/
  main.go

internal/clixcmd/
  main.go

internal/cmd/
  root.go
  check.go
  tools.go
  run.go
  approve.go
  reject.go
  runs.go

internal/cmdutil/
  factory.go

internal/iostreams/
  iostreams.go

internal/protocol/
  errors.go
  input.go
  output.go
  projection.go
  reserved.go

internal/catalog/
  catalog.go
  loader.go
  parser.go
  validate.go
  schema.go
  normalize.go

internal/domain/
  tool.go
  run.go
  status.go

internal/runservice/
  service.go
  ids.go

internal/store/
  sqlite.go
  runs.go

internal/adapter/
  registry.go
  http.go
  template.go

internal/paths/
  paths.go
```

Package responsibilities:

- `cmd/clix` is tiny. It calls `internal/clixcmd.Main` and is the only place that calls `os.Exit`.
- `internal/clixcmd` is the process-level entry point. It creates system IO streams, resolves paths, builds real dependencies, creates the command factory, executes the root command, and maps returned errors to protocol output and exit codes.
- `internal/cmd` owns Cobra command construction. It parses flags and args, builds command options, and calls application services.
- `internal/cmdutil` owns the command factory used by command constructors. It holds IO streams, protocol options, and service dependencies already constructed by `internal/clixcmd`.
- `internal/iostreams` owns stdin, stdout, stderr, and test stream helpers. `stdout` remains reserved for primary command results.
- `internal/protocol` owns the CLI contract: stable errors, input parsing, output projection, reserved flag handling, JSON failure shape, and text rendering.
- `internal/catalog` owns tool discovery, parsing, validation, JSON Schema subset checks, compiled input validators, normalization, lookup, and fingerprinting.
- `internal/domain` holds shared domain types with no dependency on Cobra, SQLite, HTTP, or YAML.
- `internal/runservice` owns run state transitions, approval checks, fingerprint checks, and adapter execution orchestration.
- `internal/store` owns persistence and database migrations.
- `internal/adapter` owns built-in adapter registration and execution.
- `internal/paths` owns user-global path resolution.

Only `internal/cmd` imports Cobra. Business packages must not import Cobra or pflag.

## Standard Streams

All commands follow standard Unix stream behavior:

- `stdout` contains only the primary result.
- `stderr` contains diagnostics and error text.
- Exit code `0` means success.
- Any non-zero exit code means failure.

Default output is concise semantic text. It is meant to be readable by humans and cheap for agents to keep in context. It is not a rich UI and does not include progress bars, spinners, colors, or long explanations.

JSON mode still uses exit codes. On failure, the JSON error object is written to `stderr`, `stdout` is empty, and the process exits non-zero.

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

`--jq` is reserved for future use. In MVP, any use of `--jq` fails with `USAGE_ERROR`.

`--dry-run` is not supported. Most real operations cannot be simulated safely, and a fake dry-run is more dangerous than no dry-run.

## CLI Command Pattern

The command layer follows the pattern used by mature Go CLIs such as GitHub CLI, Helm, and kubectl:

```text
New<Command>(factory, runF)
  -> define Cobra command and flags
  -> parse args into Options
  -> Complete derived values
  -> Validate options
  -> Run through app/service layer
```

Simple commands may collapse `Complete`, `Validate`, and `Run` into one small `RunE` handler. More complex commands, especially `run`, `approve`, and future commands with policy-sensitive behavior, must use explicit `Options`, `Complete`, `Validate`, and `Run` methods.

Rules:

- Cobra is used for command routing and flag parsing only.
- The root command sets `SilenceUsage` and `SilenceErrors` so framework defaults do not duplicate or pollute protocol output.
- Cobra output writers are wired to `internal/iostreams`.
- Cobra command suggestions are disabled in MVP. Unknown commands return the stable `USAGE_ERROR` shape, not framework-generated suggestions.
- Help, usage, and flag parse errors use custom `HelpFunc`, `UsageFunc`, and `FlagErrorFunc` wiring so command output stays inside the `clix` protocol.
- Commands return errors instead of printing and exiting.
- Error-to-output and error-to-exit-code mapping happen once at the root execution boundary.
- Command constructors accept injectable dependencies and optional `runF` functions so argument parsing can be tested without executing real side effects.
- Business logic lives in `internal/runservice`, `internal/catalog`, `internal/store`, and `internal/adapter`, not in Cobra handlers.

## Input Protocol

`clix run <tool_name>` accepts one JSON object as input.

Input can come from `--input`:

```bash
clix run weekly.get_records --input '{"week":"current"}'
```

If `--input` is absent and stdin is non-TTY, `clix` reads one JSON object from `stdin`:

```bash
make-input | clix run weekly.get_records
```

Rules:

- `--input` has explicit priority.
- If `--input` is present and stdin is a TTY, stdin is not read.
- If `--input` is present and non-TTY stdin contains non-whitespace bytes, the command fails with `USAGE_ERROR`.
- If `--input` is absent and stdin is a TTY, the command fails immediately with `VALIDATION_ERROR`.
- If `--input` is absent and stdin is non-TTY, stdin must contain exactly one JSON object.
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
- Unknown projected fields fail with `USAGE_ERROR`.
- Known projected fields that are absent on a specific result are omitted.
- Nested projection is not supported.

`--jq <expr>` is reserved for future use. In MVP, using `--jq` always fails with `USAGE_ERROR`. It is not implemented as a no-op because that would make callers believe filtering happened when it did not.

Successful JSON output is the selected command result directly. It is not wrapped in an `ok: true` envelope.

## Error Protocol

Default-mode failures write one short line to `stderr`:

```text
VALIDATION_ERROR: input.week is required
```

JSON-mode failures write a stable object to `stderr` and leave `stdout` empty:

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
MISSING_SECRET
RUN_NOT_FOUND
STORAGE_ERROR
TOOL_CATALOG_ERROR
TOOL_CHANGED
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
- A pending run can create exactly one execution attempt.
- Once a run has moved to `running`, the same run is never executed again, even if the process crashes or the adapter fails.
- `reject <run_id>` only works for `pending_approval` runs.
- `succeeded`, `failed`, and `rejected` are terminal.
- Retrying requires creating a new run.

This is a per-run execution guarantee, not an external side-effect exactly-once guarantee. External systems may still need their own idempotency controls.

Pending write runs do not expose a request preview. Default output is only run metadata:

```text
run_abcd1234 pending_approval weekly.submit_report
```

Agents that need structured metadata request it explicitly:

```bash
clix run weekly.submit_report --json id,status,tool_name,effect
```

Approval safety comes from the stored input, the tool definition, and the tool fingerprint. On `approve`, if the current tool definition is missing or its fingerprint differs from the stored fingerprint, execution is rejected with `TOOL_CHANGED` and the adapter is not executed.

## User Storage Layout

```text
~/.config/clix/
  tools/
    weekly.get_records.yaml
    weekly.submit_report.yaml

~/.local/share/clix/
  clix.db
```

Tool files are user configuration. The database is user state. Neither location is project-local in MVP.

`~` resolves to the current user's home directory. The tool directory may be absent, which means the catalog is empty. `clix` creates database parent directories when needed.

The database may contain sensitive input. It must not be committed or treated as project state.

Input retention and redaction policy are deferred outside MVP. Resolved secret values still must not be persisted in the database, included in fingerprints, or emitted in stdout, stderr, or stored error messages.

## Tool Catalog

Tool definitions live under:

```text
~/.config/clix/tools/
```

The catalog is not a single manifest file. It is the set of valid tool files discovered by recursively scanning `~/.config/clix/tools/`.

MVP discovery rules:

- Supported file extensions are `.yaml` and `.yml`.
- Files and directories whose basename starts with `.` are ignored.
- Each file defines exactly one tool.
- Tool identity comes from the file's `name` field, not from the file path.
- Tool files are loaded in deterministic path order before duplicate-name checks.
- Duplicate tool names fail catalog loading with `TOOL_CATALOG_ERROR`.
- Invalid YAML, invalid top-level shape, invalid required fields, invalid tool names, unsupported adapters, and invalid adapter configuration fail catalog loading with `TOOL_CATALOG_ERROR`.
- Cross-file imports, includes, references, inheritance, and shared fragments are not supported in MVP.

At runtime, `clix` parses each tool file into canonical JSON for validation, fingerprinting, and execution.

Validation ownership:

- `catalog` validates tool file structure, tool names, required fields, the supported JSON Schema subset, and adapter config shape.
- `catalog` stores the compiled input validator with each loaded tool.
- `runservice` validates run state, effect, approval eligibility, stored input, and tool fingerprint. It does not re-parse tool files or re-check schema structure.
- `adapter` validates execution-time adapter conditions, such as missing declared secrets.

Adapter config validation uses an interface injected into catalog loading. The catalog package does not import concrete adapter implementations, and adapter implementations do not import catalog.

## Tool File

Tool files use a strict, statically analyzable YAML subset:

- mappings, lists, strings, numbers, booleans, and null
- no custom tags
- no anchors, aliases, or merge keys
- no multi-document files

Minimal example:

```yaml
version: 1
name: weekly.get_records
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

- `version`
- `name`
- `description`
- `adapter`
- `effect`
- `input_schema`
- `output_schema`

`version` must be `1`.

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
version: 1
name: weekly.submit_report
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
tool_source_path
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
- `tool_source_path` is stored for debugging only. It is not part of tool identity.
- Public command results expose the stored input as `input`, not `input_json`.
- Adapter output is returned by the command that executes the adapter.
- Adapter output is not persisted in MVP.
- `runs get` default text output does not show input.
- `runs get --json input` may expose stored input explicitly.

The fingerprint is computed per normalized tool definition. It includes the tool name, description, effect, schemas, adapter name, adapter config, and declared secret names. It excludes the source path and resolved secret values.

## Implementation Plan

Implementation proceeds in mergeable slices. After each slice, the binary should build and the implemented command behavior should be testable.

1. CLI skeleton and protocol boundary

   Add `cmd/clix`, `internal/clixcmd`, `internal/cmd`, `internal/cmdutil`, `internal/iostreams`, and the initial `internal/protocol` error/output boundary. Implement the command tree, global `--json` and reserved `--jq` flags, root error handling, `SilenceUsage`, `SilenceErrors`, disabled command suggestions, and `clix check` with minimal catalog/store wiring.

2. Output and input protocol

   Implement JSON field projection, reserved `--jq` failure behavior, stable JSON error output on stderr, terse text output, and `clix run` input parsing from `--input` or non-TTY stdin. Enforce TTY stdin behavior and the `--input` plus non-empty non-TTY stdin conflict.

3. Tool catalog

   Implement `~/.config/clix/tools/` scanning, `.yaml` and `.yml` loading, strict YAML subset rejection, tool shape validation, name validation, duplicate detection, JSON Schema subset checking, input validator compilation, adapter config shape validation through an injected interface, canonical JSON normalization, and fingerprint generation. Implement `tools list`, `tools get`, and catalog-backed `check`.

4. SQLite run store

   Implement database path resolution, parent directory creation, migrations, run insertion, run lookup, run listing, status filtering, and terminal status updates. Use explicit transactions for approval claiming.

5. Run service

   Implement read and write run state transitions, write pending behavior, approve, reject, terminal-state guards, run ID generation, stored input handling, exactly-one execution attempt per run, and `TOOL_CHANGED` checks before adapter execution.

6. HTTP adapter

   Implement the built-in `http` adapter, simple `${input.x}` and `${secrets.X}` substitution, declared secret validation, JSON request body support, header substitution, response JSON enforcement, and `INVALID_ADAPTER_OUTPUT`.

7. Acceptance hardening

   Add CLI-level acceptance tests for the MVP scenario, failure protocol, stdout/stderr separation, reserved `--jq`, TTY stdin behavior, exactly-one execution attempt per run, and changed-tool rejection. Keep service-level tests focused on state-machine edge cases.

## Acceptance Tests

Implementation tests should focus on CLI protocol behavior, not only service internals.

Required paths:

- command construction: Cobra commands parse args into options without executing real side effects.
- `stdout` and `stderr` separation: successful primary results only go to `stdout`; default errors only go to `stderr`.
- stdin input: `clix run` reads one JSON object from non-TTY stdin when `--input` is absent.
- TTY input: `clix run` with no `--input` and TTY stdin fails immediately.
- input conflict: `--input` plus non-empty non-TTY stdin fails with `USAGE_ERROR`.
- JSON projection: `--json id,status` returns only those top-level fields.
- JSON projection errors: unknown projected fields fail with `USAGE_ERROR`.
- reserved jq: any use of `--jq` fails with `USAGE_ERROR`.
- JSON failure: failures in JSON mode write exactly `ok`, `code`, and `message` to stderr and leave stdout empty.
- catalog loading: duplicate tool names and invalid tool files fail with `TOOL_CATALOG_ERROR`.
- read execution: read tools execute immediately and return adapter output.
- write approval: write runs stop at `pending_approval`; approve creates exactly one execution attempt per run.
- tool changed: missing or changed tool definitions reject approval with `TOOL_CHANGED` before adapter execution.
- no dry-run: the command surface does not include `--dry-run`.

MVP success criteria:

- An agent can discover tools with `tools list` and `tools get`.
- An agent can request only the fields it needs with `--json`.
- An agent can compose input through stdin or pass it explicitly with `--input`.
- Errors are short, stable, and branchable by code.
- Primary results are safe to pipe because diagnostics do not pollute `stdout`.
- Write actions cannot execute before approval.
