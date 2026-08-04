# State model

## Root-qualified records

Schema 2 includes `rootId` in every mutable Skill identity. The source lock
uses `rootId + NUL + packageId`; SQLite security and group tables use composite
root keys. Version 1 records migrate to `codex-default` because older releases
managed only the Codex root. Migration does not move or rewrite Skill files.

`config.yaml` defines all absolute locations, including independent log and
report roots. `sources.lock.json` maps one
source package to one or more installed skills and records the requested ref,
resolved commit, source path and installed file hashes.

`state.db` contains append-oriented operational history:

- transactions and their status;
- security scan summaries and findings;
- explicit risk approvals.
- stable finding fingerprints, ignore decisions, reasons and timestamps.
- group labels/order in `skill_groups` and the layout-only Skill assignments in
  `skill_group_assignments`.
  - latest structured update state per root and source group in `update_statuses`.
- per-Skill security check hashes, report IDs and check times in
  `skill_security_states`.
- assisted-install parent transactions, including typed step snapshots, child
  transaction IDs, backup paths, output hashes, errors and recovery status.
- repository-wide GitHub `source_trust_policies` plus append-only
  `source_trust_audit` decisions keyed by canonical owner/repository;
- reusable `source_analyses`, `group_security_reports`, and `group_operations`
  payloads. Group operations retain a parent transaction and per-Skill child
  diagnostics while the parent status remains authoritative.

Group status values are `unknown`, `preparing`, `analyzing`,
`security-checking`, `awaiting-approval`, `installing`, `checking`,
`completed`, `partial`, `failed`, `recovery-required`, `update-available`,
`up-to-date`, `rate-limited`, and `unsupported`. A Skill-level status must not
be promoted to a separate management key.

Source-group parent transactions left `running` by an application exit are
reconciled on the next dashboard read. The parent transaction and its
`GroupOperation` become `recovery-required`, queued/running steps become
`interrupted`, and the parent rollback entry remains the recovery authority.
Completed child install transactions keep their own journals and are recovered
in reverse order by `Rollback` on the parent.

The dashboard risk count is derived from unique, non-ignored High/Critical
findings in the latest report for each in-scope installed-skill target. It is
not a count of historical scan rows.

The dashboard compares each current Skill inventory hash with
`skill_security_states`. Unchanged checked Skills are skipped by default;
missing or changed state is selected for the next security scan. A managed
Skill with a valid legacy `LastScanReport` and no local changes is treated as
checked during migration.

Installed Commit state is recorded per Skill. `PackageLock.resolvedCommit`
remains backward-compatible package metadata; readers prefer
`SkillLock.resolvedCommit` and fall back to the package value for older locks.

Generated reports are stored under `reportsRoot`; immutable pre-change content
is moved under `backupsRoot`; reversible uninstall content is moved under
`quarantineRoot`; downloads are unpacked under `stagingRoot`.

Completed read-only project scans live under `dataRoot/project-scans`. They are
bound to the source-plan ID, repository identity, context digest, canonical scan
digest, and expiry; loading by source ID returns the latest valid completed scan.

An assisted plan is bound to the project-scan ID and digest, ordinary source-plan
ID, repository identity, context digest, configuration fingerprint, plan digest
and expiry.
Runtime progress is keyed by plan/run reference and uses a monotonic sequence so
the desktop can recover the newest snapshot after navigation or dialog reopen.
Plan and progress snapshots live under `dataRoot/assisted-install`; they are
internal recovery state, not a stable public CLI file format.

The desktop stores whether its active reference points to source analysis or a
finalized plan. A non-terminal analysis snapshot without a registered backend
run after restart becomes an explicit `interrupted` terminal state. Dashboard
history merges every assisted-install transaction whose recovery status is not
`completed` into the normal recent list, so a recoverable transaction cannot be
pushed out by the 20-entry display limit.

Managed Python environments live under `dataRoot/tools/python`. MCP ownership
manifests live under `dataRoot/integrations/mcp`; the pre-change Codex
configuration is stored in the parent transaction backup. Failed managed
outputs and removed ownership records move to explicit transaction paths under
`quarantineRoot`. The transaction payload, rather than a guessed default path,
is the recovery authority.

The skill filesystem remains readable by Codex without CSM running.
