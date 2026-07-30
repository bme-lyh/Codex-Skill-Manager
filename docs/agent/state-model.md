# State model

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
- latest structured update state per source group in `update_statuses`.
- per-Skill security check hashes, report IDs and check times in
  `skill_security_states`.
- assisted-install parent transactions, including typed step snapshots, child
  transaction IDs, backup paths, output hashes, errors and recovery status.

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

An assisted plan is bound to the ordinary source-plan ID, repository identity,
context digest, configuration fingerprint, plan digest and expiry.
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
