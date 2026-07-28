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

The dashboard risk count is derived from unique, non-ignored High/Critical
findings in the latest report for each in-scope installed-skill target. It is
not a count of historical scan rows.

Installed Commit state is recorded per Skill. `PackageLock.resolvedCommit`
remains backward-compatible package metadata; readers prefer
`SkillLock.resolvedCommit` and fall back to the package value for older locks.

Generated reports are stored under `reportsRoot`; immutable pre-change content
is moved under `backupsRoot`; reversible uninstall content is moved under
`quarantineRoot`; downloads are unpacked under `stagingRoot`.

The skill filesystem remains readable by Codex without CSM running.
