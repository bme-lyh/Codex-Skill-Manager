# CLI reference

Use the executable as follows. Global options may appear before or after the
command:

```text
csm [--config ABSOLUTE_PATH] [--json] COMMAND
```

`--json` returns one envelope with `schemaVersion`, `command`, `status`,
optional `data`, and optional `error`. This includes `csm --json version`.
Running `csm` without a command prints the built-in command list; there is no
separate `help` command.

## Inspect

```powershell
csm discover
csm dashboard
csm audit
csm audit --skill "skill-name"
csm check [--group "github:owner/repository" ...] [--force]
csm github-auth
csm codex status
csm codex review --report "scan-..." [--skill "skill-name" ...]
csm history
csm reports
csm doctor
csm version
```

Without `--skill`, `audit` scans all non-system Skills. `check` accepts repeated
`--group` values; `--force` bypasses the short-lived GitHub cache.

## Manage existing Skills and groups

```powershell
csm bootstrap
csm manage --skill "skill-a" --skill "skill-b"
csm manage --plan-id "manage-plan-..." --skill "skill-a" --apply

csm group create --name "Daily tools"
csm group rename --id "group-..." --name "Development"
csm group reorder --id "group-a" --id "group-b"
csm group move --group "group-..." --skill "skill-a" --skill "skill-b"
```

`manage` first creates a plan. Applying that plan records source and snapshot
metadata without moving the existing Skill directory.

## Standard installation and updates

Create and review a plan, then apply it with explicit Skill names:

```powershell
csm --json install --url "https://github.com/owner/repository" [--ref "tag-or-commit"]
csm --json install --local "D:\skills\package"
csm --json install --plan-id "plan-..." --assess
csm --json install --plan-id "plan-..." --skill "skill-a" --skill "skill-b" --apply

csm check
csm --json update --group "github:owner/repository"
csm --json install --plan-id "update-plan-..." --skill "skill-a" --apply
```

Standard installation installs only selected Skill directories. It does not
install extra tools or change Codex MCP configuration.

## Codex assisted installation

Assisted installation has three explicit phases:

```powershell
csm --json install --url "https://github.com/owner/repository" --assist
csm --json install --assist --project-scan-id "project-scan-..." --create-plan
csm --json install --assist --plan-id "assisted-plan-..." --apply `
  --skill "skill-name" `
  --grant "PERMISSION_ID" `
  --project-root "D:\work\project"
```

The first command returns a read-only project overview, advisory security
conclusion, evidence coverage, and declarative installation methods. Review it
before running the second command, which is the explicit consent to create a
plan. Then review the returned Skills, steps, warnings, permissions, and
`needsProjectRoot`. On apply, pass only permission IDs listed by that plan.
`--all` may replace repeated `--skill` values. Supply `--project-root` only
when the approved MCP plan requires it; the path must be a real Git or SVN
working tree. Project scanning does not download dependencies or create an
installation plan, and plan creation cannot be combined with `--apply`.

Using `--assist` explicitly opts in for that invocation. It uses the configured
Codex CLI, model, reasoning effort, and account usage. It does not enable
arbitrary repository scripts or free-form generated commands.

## Remove, restore, and roll back

```powershell
csm remove "skill-a" "skill-b"
csm restore --skill "skill-a" --transaction "tx-..."
csm rollback --transaction "tx-..."
```

`remove` accepts positional Skill names, moves them to quarantine, and does not
accept `--skill`, `--apply`, wildcards, or `--all`. Use `history` to find
transaction IDs.

## Warning decisions and scheduling

```powershell
csm warning --fingerprint "HASH" --rule "RULE_ID" --file "skill/SKILL.md"
csm warning --cluster "RISK_ID" --fingerprint "HASH1" --fingerprint "HASH2"
csm warning --report "scan-..." --dry-run
csm warning --report "scan-..."
csm warning --report "scan-..." --restore

csm schedule --enabled=true --frequency=weekly --at=09:00
```

Critical warnings cannot be ignored. Accepting a High cluster requires a
non-empty `--reason` and `--confirm-deterministic`; lower severities keep an
optional reason. Applying a plan with an accepted High cluster also requires
`--accept-high-risk` as a final acknowledgement; that flag cannot create the
decision by itself. Use `--restore` to reverse an accepted or ignored warning.
Report-wide ignore processes only known Medium-or-lower clusters and returns
High, Critical, or unknown entries in `skippedClusterIds`; report-wide restore
may reverse matching persisted decisions at any known severity.
Schedule frequency is `daily` or `weekly`, and time uses 24-hour `HH:mm`.

Exit code `0` means success, `1` means an operational or policy failure, and
`2` means invalid command usage. Invalid flags and missing required options
stop before the requested operation runs.

See the [Chinese CLI reference](../user/cli-reference.md) and the
[Agent command contracts](../agent/command-contracts.md).
