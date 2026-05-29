# clix MVP Technical Design v0.1

## Architecture

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

Adapters are built into `clix`. MVP does not include an external adapter protocol.

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

## Output Pipeline

All commands write their primary result to stdout and diagnostics to stderr.

The output layer owns three output modes:

- Default text output for concise semantic summaries.
- `--json <fields>` projection for selected top-level fields.
- `--jq <expr>` filtering after JSON projection.

`--jq` implies JSON output. If both `--json` and `--jq` are present, field projection runs before the jq expression.

Failures in JSON mode return a stable error object:

```json
{
  "ok": false,
  "code": "VALIDATION_ERROR",
  "message": "..."
}
```
