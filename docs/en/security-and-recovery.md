# Security and recovery

Every source first passes a required local assessment. The optional Enhanced
project scan stays read-only until you approve its plan and permissions.
Critical findings cannot be ignored. High findings require confirmation and a
reason for each cluster. Report-wide ignore handles known Medium or lower
findings only.

All downloaded Skill content is untrusted. The manager never executes repository scripts and never modifies either root's `.system` directory.

The built-in scanner checks prompt injection, credential access, dynamic execution, download-and-execute patterns, destructive commands, persistence, external URLs, obfuscation, global Codex configuration changes, symbolic links, path escape, file size, and unsupported file types.

| Severity | Default behavior |
|---|---|
| Informational / Low | Record |
| Medium | Display for review |
| High | Block writes; only per-cluster confirmation with a non-empty reason may accept it |
| Critical | Always block writes and cannot be ignored |

Before replacement, the current Skill is backed up and the operation is journaled. Removal moves one explicitly selected Skill at a time to quarantine. Recovery is available from **History** or **Quarantine**.

When the Skills root and the configured backup directory are on different Windows drives, backup and quarantine content is kept in `.csm-backups` or `.csm-quarantine` under the Skills root. This preserves same-volume atomic moves. The transaction journal records the actual path, and these internal directories are excluded from inventory and scanning.

Identical same-name Skills duplicated across repository packaging paths are de-duplicated. Different content using the same Skill name is rejected and requires an explicit repository path. File-count errors report both the selected total and the configured total limit.

Large reports are de-duplicated by stable fingerprint and capped in the GUI for responsiveness. The complete raw findings remain available in the local JSON report.

Install, update, management, and security views show a five-level summary.
Medium and lower clusters use the ordinary ignore flow. High requires explicit
per-cluster confirmation and a non-empty reason. Critical cannot be ignored,
and batch actions cannot accept High or Critical. Every decision is atomic and
journaled. The backend resolves the persisted finding and severity immediately
before apply instead of trusting client risk metadata.

Static analysis is best-effort and cannot prove that content is safe. Review source ownership, the resolved commit, requested permissions, and every high-impact finding.

Every installation source also passes a persisted layered assessment before any
optional Codex work: pin a GitHub commit or create a bounded manager-owned local
snapshot, read documentation and installation markers, classify the project,
discover Codex Skills, scan exact targets, and group checks as required,
triggered, or optional. Apply recomputes this assessment from the bound source.
Changed, expired, digest-mismatched, unknown, unsupported, and case-variant
`.system` targets fail closed.
Discovered Skills install under the configured Skills root, which defaults to
`%USERPROFILE%\.codex\skills` or `%USERPROFILE%\.agents\skills`; each `.system`
directory is always read-only. A project with
no Codex Skill is not copied to that global folder. Unsupported work remains
manual, and an MCP project directory must be selected explicitly.

Optional Codex risk review requires an independently executable and
authenticated CLI. The application probes every PATH candidate plus the
current user's npm directory, skipping a WindowsApps executable that
third-party processes cannot launch. A manually configured CLI path must be
absolute and executable. Compatibility is capability-based and does not pin a
specific CLI version. After authentication, the model picker is populated from
the visible catalog returned by the current CLI. The app packages the complete
selected group into model input, disables shell access, and uses a manager-owned
output directory as the working directory. Local rule clusters are supplemental
leads. The user may accept applicable Codex suggestions or use the ordinary
human decision flow for eligible findings.

Planned installation is a separate, opt-in workflow. It first creates
the normal commit-pinned GitHub or explicit-local source preview and local scan,
then packages the complete prepared source for Codex with shell access disabled
and a manager-owned output directory as the working directory. Codex can
propose only declarative actions. A local finalizer derives permissions and
allows automatic execution only for selected Skill installation, an
exact-version verified Python tool from official PyPI, and a manager-owned Codex
MCP entry. Repository scripts, arbitrary shell commands, environment changes,
and unsupported package managers remain manual. Managed Python and MCP
automation requires a GitHub source so official PyPI metadata can verify package
ownership. Approved automatic steps can complete first; the transaction then
reports `partial` and lists the remaining manual work.

Python dependency resolution happens during the explicitly requested analysis.
The app downloads the complete Wheel closure from official PyPI into isolated
staging and locks each distribution name, version, filename, and SHA256. It
does not install or run packages at that stage. Source distributions, direct
URLs, incompatible artifacts, and unsafe archives are rejected. Native-code
Wheels require a separate high-risk permission. Apply verifies the cached
files and installs offline with hashes required. App-launched pip is forced
through a temporary local proxy that permits only the two official PyPI HTTPS
hosts. Inherited proxy and pip network settings are removed, and every
downloaded filename, URL, and hash must match official PyPI metadata. This is
application-level egress control rather than an operating-system sandbox, so
the local Python and pip installation must still be trusted.

The Settings toggle controls Security Center risk review only. Choosing the
Enhanced project scan in the GUI or using CLI `--assist` is the explicit opt-in
for that installation. Both use the configured CLI path, model, and reasoning
effort and consume Codex usage.

Each approved automatic step is journaled with explicit targets, backup paths,
and output hashes. Failure triggers reverse-order recovery for completed
reversible steps. Recovery refuses to overwrite a Codex configuration or
managed output that changed after installation and instead reports the backup
and manual recovery path. Manual steps were never executed and are outside the
automatic rollback scope.
