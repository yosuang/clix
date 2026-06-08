<div align="center">
  <h1>clix</h1>
  <p><em>clix turns messy operational actions into typed, permissioned, auditable tools for humans and AI agents.</em></p>
</div>

## MVP Commands

```bash
clix check
clix tools list
clix tools get <tool_name>
clix run <tool_name> --input '<json>'
clix approve <run_id>
clix reject <run_id>
clix runs list
clix runs get <run_id>
```

Tool definitions live under `~/.config/clix/tools/`. Run state is stored in `~/.local/share/clix/clix.db`.
