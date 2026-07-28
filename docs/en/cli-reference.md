# CLI reference

Run `csm help` for the authoritative command list. Add `--json` to receive structured output suitable for Agents and automation.

```powershell
csm doctor
csm discover
csm dashboard
csm scan --skill skill-name
csm check
```

Mutations use a two-step contract:

```powershell
csm --json install --url "https://github.com/owner/repository"
csm install --plan-id "plan-..." --skill "skill-name" --apply
```

Existing unmanaged Skills:

```powershell
csm manage --skill "skill-name"
csm manage --plan-id "adopt-plan-..." --skill "skill-name" --apply
```

Updates:

```powershell
csm check
csm update --group "github:owner/repository"
csm install --plan-id "plan-..." --skill "skill-name" --apply
```

Removal is recoverable:

```powershell
csm remove --skill "skill-name" --apply
csm quarantine list
csm restore --skill "skill-name" --transaction "tx-..." --apply
```

See the complete Chinese [CLI reference](../user/cli-reference.md) and the language-neutral [command contracts](../agent/command-contracts.md).
