# Security and recovery

All downloaded Skill content is untrusted. The manager never executes repository scripts and never modifies `.codex/skills/.system`.

The built-in scanner checks prompt injection, credential access, dynamic execution, download-and-execute patterns, destructive commands, persistence, external URLs, obfuscation, global Codex configuration changes, symbolic links, path escape, file size, and unsupported file types.

| Severity | Default behavior |
|---|---|
| Informational / Low | Record |
| Medium | Display for review |
| High | Block writes by default; a human ignore decision may clear it |
| Critical | Block writes by default; a human ignore decision may clear it |

Before replacement, the current Skill is backed up and the operation is journaled. Removal moves one explicitly selected Skill at a time to quarantine. Recovery is available from **History** or **Quarantine**.

When the Skills root and the configured backup directory are on different Windows drives, backup and quarantine content is kept in `.csm-backups` or `.csm-quarantine` under the Skills root. This preserves same-volume atomic moves. The transaction journal records the actual path, and these internal directories are excluded from inventory and scanning.

Identical same-name Skills duplicated across repository packaging paths are de-duplicated. Different content using the same Skill name is rejected and requires an explicit repository path. File-count errors report both the selected total and the configured total limit.

Large reports are de-duplicated by stable fingerprint and capped in the GUI for responsiveness. The complete raw findings remain available in the local JSON report.

Install, update, management, and security views show a five-level summary. A human may ignore one cluster or every active cluster in the report in one action. Reasons are optional, deterministic rules use the same flow, and every batch is atomic and journaled. The backend reloads ignore decisions immediately before apply; active High/Critical findings block, while ignored findings no longer participate in the gate.

Static analysis is best-effort and cannot prove that content is safe. Review source ownership, the resolved commit, requested permissions, and every high-impact finding.

Optional Codex semantic review requires an independently executable and
authenticated CLI. The application probes every PATH candidate plus the
current user's npm directory, skipping a WindowsApps executable that
third-party processes cannot launch. A manually configured CLI path must be
absolute and executable. Compatibility is capability-based and does not pin a
specific CLI version. After authentication, the model picker is populated from
the visible catalog returned by the current CLI. Codex runs with the complete
target directory as a read-only working context, inventories the repository,
and reads relevant implementation, scripts, tests, examples, and documentation.
Local rule clusters are supplemental leads. The user may accept applicable
Codex suggestions or use the independent human ignore-all action.
