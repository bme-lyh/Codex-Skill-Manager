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
is bound to an immutable GitHub Commit and includes a scan. Group installation
and update require the complete source-group member set, create one parent
transaction, and record each Skill only as a child step for progress, backup,
failure, and recovery details.

Warning ignore and restore decisions are journaled as `ignore-warning` and
`restore-warning` transactions. Their recovery path is the inverse warning
action; generic transaction rollback supports `install`, legacy `adopt`,
`manage`, and `group-*`.

## Planned installation

Assisted execution creates an `assisted-install` parent transaction before any
step runs. Apply revalidates the source-bound plan, complete source-group Skills, plan
and configuration digests, expiry, explicit permission IDs, and optional
project root. Each typed step moves through queued/running/completed/failed or
skipped states in the transaction payload:

- `install-skills` delegates to the ordinary installation transaction and keeps
  its child transaction ID and backups;
- `managed-python-tool` carries the approval-time complete Wheel lock, then
  records its explicit managed root, verified Wheel hashes and quarantine
  recovery path;
- `configure-codex-mcp` records the configuration backup, applied hash and
  ownership manifest;
- `manual` is reported but never mutated or marked as automatically completed.

Before approval, every managed Python step verifies the root package's GitHub
ownership through official PyPI metadata, resolves a bounded Wheel-only
dependency closure, validates each archive, and binds every filename and hash
to `planDigest`. Resolution failure converts the tool and dependent MCP step to
required manual work. Before the first mutation, apply checks that the cached
Wheel set still matches the lock. Installation is offline and uses pip
`--require-hashes`; it never changes the approved dependency graph at runtime.
Platform-specific Wheels also require the derived `managed-native-code`
permission.

Required manual steps become `manual-pending`. If at least one supported step
is approved, those steps run transactionally and the parent plan, transaction,
and terminal progress use `partial` with `recoveryStatus: "available"`.
`manual-pending` counts as processed for progress but never as automatically
completed. A partial run may be rolled back but cannot be automatically retried.
The approval journal and plan persist the exact source-group member set and
canonical project root before the first mutation.

If a supported step fails or the user cancels at a safe point, the manager
attempts recovery for completed reversible steps in reverse order and persists
the result before returning. Recovery checks the applied output hash first. A
hash mismatch means the user or another process changed the target; the manager
must not overwrite it and instead marks recovery as incomplete with an exact
manual path. A retry is allowed only after recovery completed and must not
silently repeat an unrecovered non-idempotent mutation.

The generic desktop rollback entry for the parent transaction uses the same
step-specific recovery handlers. Reports include both the original execution
failure and any recovery failure so a partial rollback cannot be presented as
success.
