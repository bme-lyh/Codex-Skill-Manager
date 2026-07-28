# Security and recovery

All downloaded Skill content is untrusted. The manager never executes repository scripts and never modifies `.codex/skills/.system`.

The built-in scanner checks prompt injection, credential access, dynamic execution, download-and-execute patterns, destructive commands, persistence, external URLs, obfuscation, global Codex configuration changes, symbolic links, path escape, file size, and unsupported file types.

| Severity | Default behavior |
|---|---|
| Informational / Low | Record |
| Medium | Display for review |
| High | Require explicit acceptance |
| Critical | Block by default; clear only after each finding is reviewed and ignored with a recorded reason |

Before replacement, the current Skill is backed up and the operation is journaled. Removal moves one explicitly selected Skill at a time to quarantine. Recovery is available from **History** or **Quarantine**.

When the Skills root and the configured backup directory are on different Windows drives, backup and quarantine content is kept in `.csm-backups` or `.csm-quarantine` under the Skills root. This preserves same-volume atomic moves. The transaction journal records the actual path, and these internal directories are excluded from inventory and scanning.

Identical same-name Skills duplicated across repository packaging paths are de-duplicated. Different content using the same Skill name is rejected and requires an explicit repository path. File-count errors report both the selected total and the configured total limit.

Large reports are de-duplicated by stable fingerprint and capped in the GUI for responsiveness. The complete raw findings remain available in the local JSON report.

Install, update, and security views show a five-level summary. Every unique finding can be loaded, reviewed, ignored with a mandatory reason, and restored. The backend reloads ignore decisions immediately before apply; active Critical findings still block, while reviewed and ignored findings no longer participate in the gate.

Static analysis is best-effort and cannot prove that content is safe. Review source ownership, the resolved commit, requested permissions, and every high-impact finding.

Optional Codex semantic review requires an independently executable and
authenticated CLI. The application probes every PATH candidate plus the
current user's npm directory, skipping a WindowsApps executable that
third-party processes cannot launch. A manually configured CLI path must be
absolute and executable. Compatibility is capability-based and does not pin a
specific CLI version. After authentication, the model picker is populated from
the visible catalog returned by the current CLI. After reading the review, the
user can accept applicable suggestions in one action. Deterministic baselines
remain subject to an additional explicit human confirmation, and every cluster
decision is journaled separately.
