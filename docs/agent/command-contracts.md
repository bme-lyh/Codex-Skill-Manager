# Command contracts

Global options are accepted before the command:

```text
csm --config <absolute-config-path> --json <command>
```

Root-scoped commands accept `--root codex-default|agents`. Compatibility calls
default to `codex-default`, but automation should always send an explicit root.
Plans seal `targetRootId`, and apply rejects a different root.

Read-only commands:

- `doctor`, `dashboard`/`discover`, `audit [--skill NAME]`, `check`;
- `history`, `reports`, `version`.

`audit` without `--skill` explicitly scans every non-system Skill and records
application group metadata. With `--skill`, it uses the same selective scan path
for that named Skill.

`check` accepts repeatable `--group` and optional `--force`, then persists and
returns structured per-source status (`up-to-date`, `update-available`,
`unsupported`, `rate-limited`, or `error`) plus timestamps, remote Commit,
per-Skill current Commits, and explicit outdated Skill names.

`github-auth` validates credential/quota state. `codex status` is read-only and
returns authentication plus capability-based compatibility without a version pin;
after authentication it also returns the visible model catalog reported by the
current CLI, with catalog failures kept separate from compatibility failures.
`codex review --report ID [--skill NAME ...]` creates a journaled advisory
review. Repeated `--skill` values are retained for compatibility, but new group
operations review every valid Skill discovered inside the persisted report target.
The result is stable per Skill. All Skills in the same application group share
one review task. Groups execute serially by default; a failed group or
an output missing a requested Skill is retried once serially.
If at least one group succeeds, the command returns the partial report; its
review and journal status are `partial`, with failed groups named explicitly.

Planning commands:

- `update --group ID`: resolve a GitHub source and create a persisted update
  preview for its installed Skills; scan scope is limited to actual candidate
  Skill directories;

Source-group apply surfaces use the immutable group membership sealed in the
preview. A group install/update must submit every valid member in that
preview; submitting only a subset is an explicit validation error. The legacy
selective install facade remains source-compatible for older clients, while
new group callers use the parent transaction and child diagnostics contract.

The desktop and CLI group surfaces are `GetOrCreateSourceGroupAnalysis`,
`RunGroupSecurityCheck`, `ApproveGroupSecurity`, `ApproveGroupRisk`,
`PrepareGroupUpdate`, `ApplyGroupInstall`, and `ApplyGroupUpdate`. `--approve-risk`
is the CLI equivalent of the one-click group decision and never changes the
immutable source or technical gates.

`GetGroupOperation` (`csm group operation --id ID`) returns one persisted
source-group parent operation with its step diagnostics. `GetGroupMetadata`
(`csm group metadata --group ID [--root ROOT_ID]`) returns the dashboard group
plus the latest analysis, security report, operation, and update status in one
read-only contract. Both are safe to call at any time and never mutate state.

Reusable group approvals are bound to the exact `GroupSecurityReport` plus its
root, group, repository, commit, and `policyVersion`. A newer report or a plan
for a different commit/repository cannot reuse an older approval; callers must
approve the current report again. Legacy reports without a policy version keep
the v0.14 group-prefix approval key only after the same binding checks pass.

Source-group parent transactions that remain `running` after an application
exit are reconciled on the next dashboard read: the parent and its group
operation become `recovery-required`, queued/running steps become
`interrupted`, and the parent rollback entry remains the recovery authority.

Repository trust is managed by canonical GitHub `owner/repository` identity.
Set/revoke decisions are journaled and append an audit row. Trust does not
replace immutable-ref, local scanner, path/hash, or recovery checks. Critical
group risk may be approved with no reason after a persisted decision, but all
technical integrity gates still apply.

- `install --url URL [--ref REF] [--assist]`: create a commit-pinned preview;
  when `--assist` is present, enforce the mandatory assessment and continue to
  a reusable read-only Codex project scan;
- `install --local PATH [--assist]`: copy the explicit directory into a bounded,
  manager-owned snapshot, then use the same explicit assessment contract;
- `install --plan-id ID --assess`: persist and return the mandatory read-only
  layered assessment for review before apply;
- `install --assist --project-scan-id ID --create-plan`: explicitly approve
  creation of a typed assisted-install plan from a verified scan.

Without `--assist`, the CLI `install` contract remains Skill-only. It never
installs dependencies or edits Codex MCP configuration.

## Assisted-install contract

The desktop facade and CLI expose the same manager workflow. They must preserve
this sequence:

1. create the ordinary source preview and local scan;
2. create a reusable project scan from local results, complete bounded file
   summaries, and deterministic focused-file analysis;
3. show the overview, security conclusion, evidence limitations, and
   declarative installation methods without creating an installation plan;
4. only after explicit user approval, package the verified scan and prepared
   commit-pinned GitHub staging directory or explicit
   local source into an opt-in Codex session with the shell tool disabled;
   oversized input is covered by deterministic no-tools chunks and a final
   synthesis;
5. validate the schema locally, bind it to the project-scan/source/configuration/plan
   digests, downgrade unsupported actions to `manual`, and derive permissions;
6. show the summary, requirements, warnings, exact Skills, typed steps, and
   every required permission;
7. apply only the exact complete source-group Skills and approved permission IDs, plus an
   explicit project root when MCP requires one;
8. resolve any approved Python tool's complete Wheel closure during plan creation,
   record exact package identities and hashes, and reject source distributions;
9. journal each step and persist monotonic progress, cancellation, retry, and
   recovery; the desktop relays live progress while CLI returns the final
   structured state.

Supported executable step kinds are `install-skills`,
`managed-python-tool`, and `configure-codex-mcp`. `manual` is never executed.
The executor must reject missing permissions, expired or changed plans,
unselected targets, changed Codex configuration, model-supplied paths or
environment, non-Wheel Python builds, an existing unmanaged MCP name, and an
MCP project root that is not a real Git or SVN working tree.
Managed Python execution requires a GitHub source whose repository identity
matches official PyPI metadata. Apply verifies the staged Wheel lock and runs
offline with hashes required; native-code Wheels need a separate high-risk
permission. Approved automatic steps may finish before a `partial` result lists
required manual work; optional manual work is shown and skipped.
The immutable Wheel approval permission ID is `pypi-wheel-lock`; it does not
grant apply-time network access. `selectedSkills` is persisted as the exact
approved subset and must default to empty, never all candidates, when absent.

Codex output cannot authorize arbitrary commands. Repository scripts are never
run. On failure, completed reversible steps recover in reverse order. If output
hashes no longer match, recovery refuses to overwrite the changed file and
returns an explicit recovery status and instructions.

CLI project scanning uses `install --url URL --assist` or
`install --local PATH --assist`. Plan creation requires
`install --assist --project-scan-id ID --create-plan`. Apply uses
`install --assist --plan-id ID --apply --skill NAME... --grant ID...`, with
optional `--all` and a required `--project-root PATH` only when the plan says it
needs MCP. `--assist` itself is the per-invocation opt-in; it does not depend on
the Security Center Codex-review toggle. It still uses the configured CLI path,
model, reasoning effort, and account usage. Internal plan snapshots are not a
public file interface.

Mutating commands:

- `bootstrap`: manage the two known existing packages without replacing files;
- `manage --skill NAME...`: create a persisted source-detection and scan plan;
- `manage --plan-id ID --apply --skill NAME...`: apply an explicit plan;
- `group create --root ROOT_ID --name NAME`;
- `group rename --id ID --name NAME`;
- `group reorder --id ID...`;
- `group move --group ID --skill NAME...`;
- `install --root ROOT_ID --plan-id ID --apply --skill NAME... [--accept-high-risk]`;
- assisted apply: `install --assist --plan-id ID --apply --skill NAME... --grant ID... [--project-root PATH]`;
- `remove --root ROOT_ID NAME...`: move explicit names into quarantine;
- `restore --root ROOT_ID --skill NAME --transaction ID`;
- `rollback --transaction ID`;
- `warning --fingerprint HASH --rule ID --file PATH [--reason TEXT]`:
  persist an ignore decision; `--restore` removes it;
- `warning --cluster ID --fingerprint HASH...`: apply one human decision to a
  complete cluster;
- `warning --report SCAN_ID [--dry-run] [--restore]`: preview or atomically
  apply one human decision to every eligible matching cluster in a report;
  ignore filters to known Medium-or-lower severities and returns skipped IDs,
  while restore may reverse matching persisted decisions at any known severity;
- `schedule`: create or update a scheduled check.

Names must be explicit and cannot contain wildcards. `.system` is reserved
case-insensitively in every mutation route. A plan expires after 24 hours.
For a source-group operation, Critical, High, Medium, and Low findings may be
approved by one explicit human action without a typed reason. The approval is
bound to the group report/plan and never bypasses immutable commit, path,
snapshot, hash, or recovery checks. Legacy Skill-level warning commands retain
their narrower compatibility rules. `--accept-high-risk`, `--deterministic`, and
`--confirm-deterministic` remain accepted for compatibility; the last flag is the
explicit High-risk decision confirmation. `--accept-high-risk` is a separate final
apply acknowledgement when a persisted High decision exists and cannot create or
bypass that decision. Callers must preserve the returned transaction ID.

Managing existing Skills is metadata-only: it snapshots `sources.lock.json`,
detects sources, hashes the current files and records a `manage` transaction.
Legacy `adopt` remains an alias. Group mutations snapshot `groups.json` and
never mutate source provenance or Skill content.
Finding/cluster decisions are reloaded from local state at apply time. Every
decision creates a transaction record. Group approvals do not require a reason;
restored findings and changed snapshots immediately participate in the gate
again. A Codex verdict can never substitute for the explicit human decision.
A Codex verdict can never substitute for the explicit human decision.
