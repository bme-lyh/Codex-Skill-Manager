# CLI contract

Invoke:

```powershell
csm --config <absolute-path> --json <command>
```

Safe inspection:

```powershell
csm --json dashboard
csm --json audit
csm --json check
csm --json history
csm --json version
csm --json codex review --report SCAN_ID --skill SKILL_A --skill SKILL_B
```

Without `--skill`, `audit` scans all non-system Skills and preserves their
application group metadata. Use `--skill NAME` for one explicit Skill.

Codex review is optional and journaled. Repeated `--skill` values restrict the
review to trusted Skills inside the persisted scan target; omitting them reviews
all detected Skills. Read `skillReviews` per Skill and inspect `batches` for
partial group failures. Selected Skills in the same application group share one
review task. Groups are serial by default, and a failed or incomplete group is
retried once. Local rules are sent only as a compact count overview.

Two-phase GitHub installation:

```powershell
csm --json install --url https://github.com/OWNER/REPO
csm --json install --plan-id PLAN_ID --apply --skill SKILL_NAME
```

Local installation uses `install --local ABSOLUTE_PATH`, followed by the
same apply command. Repeat `--skill` to select multiple skills.

Two-phase Codex assisted installation:

```powershell
csm --json install --url https://github.com/OWNER/REPO --assist
csm --json install --assist --plan-id ASSISTED_PLAN --apply `
  --skill SKILL_NAME --grant PERMISSION_ID
```

Use `--all` instead of repeated `--skill` when the user approved every candidate.
Repeat `--grant` only for permission IDs shown by the plan. Add
`--project-root ABSOLUTE_GIT_OR_SVN_PATH` only when `needsProjectRoot` is true.
Creating a plan cannot be combined with `--apply`.

The `--assist` flag is the explicit opt-in for that installation. It is
independent of the Security Center risk-review toggle but uses the configured
Codex CLI, model, reasoning effort, and account usage. Read the structured
summary, requirements, steps, warnings, permissions, and recovery fields.
Codex output cannot authorize arbitrary commands. Approved automatic steps may
finish before a `partial` result reports required manual work; internal plan
files are not a public interface.

Two-phase management of existing unmanaged Skills:

```powershell
csm --json manage --skill SKILL_A --skill SKILL_B
csm --json manage --plan-id MANAGE_PLAN --apply --skill SKILL_A --skill SKILL_B
```

Layout-only groups:

```powershell
csm --json group create --name "Development"
csm --json group rename --id GROUP_ID --name "Daily tools"
csm --json group reorder --id GROUP_A --id GROUP_B
csm --json group move --group GROUP_ID --skill SKILL_A
```

Ignore or restore an exact warning:

```powershell
csm --json warning --fingerprint HASH --rule RULE_ID --file FILE
csm --json warning --fingerprint HASH --rule RULE_ID --file FILE --restore
csm --json warning --report SCAN_ID --dry-run
csm --json warning --report SCAN_ID
```

Reasons are optional. Report-wide decisions apply atomically to every active
cluster of every severity, including deterministic rules.

Reversible lifecycle:

```powershell
csm --json remove SKILL_A SKILL_B
csm --json restore --skill SKILL_A --transaction TRANSACTION_ID
csm --json rollback --transaction TRANSACTION_ID
```

Exit 0 means success, 1 means operational or policy failure, and 2 means invalid
usage. JSON output has `schemaVersion`, `command`, `status`, optional `data`,
and optional `error`, including for `version`. Invalid flags, unexpected
arguments, and missing required options return 2 before the requested operation
runs.
