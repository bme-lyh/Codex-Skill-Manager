# Transaction model

Every mutation receives a unique transaction ID and follows:

1. validate names, source and security policy;
2. persist a running transaction;
3. snapshot `sources.lock.json`;
4. move existing content to a transaction-specific backup when needed;
5. copy or move the requested content;
6. hash the final files and commit the source lock;
7. mark completed and write Markdown/JSON reports.

Multi-skill installs and quarantines are atomic at the application level. On a
failure, processed skills are restored in reverse order and the source-lock
snapshot is restored. Failed new content is retained in quarantine for
forensics. No rollback path requires recursive deletion.

Managing existing Skills is a metadata-only mutation. Its plan contains
explicit unmanaged skill names, current file hashes, detected source evidence
and a scan report. Applying it snapshots the source lock, records a `manage`
transaction, and writes separate package mappings for detected sources.
Rollback restores only the source-lock snapshot and never moves Skill content.

Group create, rename, reorder and move operations record `group-*`
transactions. Each snapshots the complete layout to `groups.json`; rollback
restores that database layout and never changes `sources.lock.json` or Skill
files.

Interactive updates reuse the install plan and transaction model. The preview
is bound to an immutable GitHub Commit and includes a scan. Applying a selected
subset backs up only those explicit Skills and stores the new Commit on each
selected `SkillLock`; unselected Skills retain their previous Commit.

Warning ignore and restore decisions are journaled as `ignore-warning` and
`restore-warning` transactions. Their recovery path is the inverse warning
action; generic transaction rollback supports `install`, legacy `adopt`,
`manage`, and `group-*`.
