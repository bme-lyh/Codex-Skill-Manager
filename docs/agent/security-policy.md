# Security policy

CSM is deny-by-default at the mutation boundary:

- only `https://github.com` and explicit local directories are accepted;
- archives reject path traversal and symbolic links;
- file count and file-size limits are enforced;
- `.system` is excluded and protected;
- static scanning runs before an installation plan is applyable;
- active High/Critical findings block writes until the source-group decision is
  persisted; findings are clustered by rule and file class;
- group operations allow one human approval for Critical, High, Medium, and Low
  without a typed reason, while immutable refs, paths, hashes, snapshots, and
  recovery checks remain mandatory;
- legacy report-wide decisions may still be limited to eligible Medium-or-lower
  clusters for compatibility;
- batch decisions are persisted atomically and journal explicit cluster targets;
- Codex review is advisory and never records a human decision by itself;
- source commits and installed file hashes are recorded;
- GitHub secrets are not written to config, logs or reports.

Local scanning is the default. Codex CLI review is opt-in and packages the full
selected group into model input. Text is included verbatim, binary files use
immutable metadata, the shell tool is disabled, and the working directory
contains only manager-owned output. Local rule clusters are supplemental leads. The ephemeral session strips common
secret environment variables, disables approval and returns schema-validated
output. Repository text remains untrusted data; only an explicit human action
changes ignore state.

Planned installation is also opt-in and must start from the ordinary
commit-pinned GitHub or explicit-local, scanned source preview. Its first phase
is a reusable read-only project scan: local findings, bounded summaries for the
complete eligible text inventory, then deterministic focused-file analysis.
Credential-like files are metadata-only and large text is truncated before
cloud submission. The session disables the shell tool and runs from a
manager-owned output directory. The scan returns only an overview, advisory
security conclusion, and declarative installation methods.

Project scanning is not installation consent. Plan creation requires a second
explicit user action and binds the resulting plan to the scan ID, scan digest,
source digest, and expiry. A sensitive capability that is necessary for the
declared Skill may be reported as contextual with a warning; it is not
automatically classified as harmful or granted execution authority.

The global `codexReview.enabled` setting gates Security Center risk review only.
Selecting the assisted mode in the desktop or passing CLI `--assist` is the
per-install opt-in and still uses the configured CLI path, model, reasoning
effort, and account usage.

Local code must derive permissions and restrict execution to:

- exact complete source-group Skill installation through the normal manager;
- an exact-version, repository-matched Python package whose complete Wheel-only
  dependency closure is resolved from official PyPI before approval;
- a new manager-owned Codex MCP entry that invokes that managed executable only
  with the fixed `serve` argument and an explicit version-controlled project
  root.

Every resolved Wheel is identified by project, version, filename, compatibility
tags, and SHA-256 in the plan digest. Pure Wheels use the ordinary managed-tool
permissions. Platform-compatible native Wheels also derive the explicit
`managed-native-code` high-risk permission and list each affected Wheel and
hash. Unknown or incompatible native platforms remain manual.

The resolver must clear inherited proxy and `PIP_*` network configuration and
force app-launched pip through a temporary loopback CONNECT proxy restricted to
`pypi.org:443` and `files.pythonhosted.org:443`. TLS remains end-to-end in pip.
Each accepted artifact must match official PyPI metadata by normalized project,
version, filename, URL, and SHA-256. This is process configuration, not an
OS-level network sandbox; the trust boundary therefore includes the selected
local Python and pip executables.

Apply never resolves dependencies again. It verifies that the cached Wheel set
exactly matches the approved lock, creates the managed environment, and invokes
pip with `--no-index --require-hashes`. If official metadata, repository
ownership, the complete Wheel closure, or the bounded cache cannot be verified
during analysis, the integration and dependent MCP action become required
manual steps instead of falling back to a dynamic runtime install.

Repository scripts, arbitrary shell, source package builds, model-selected
paths, model-selected environment variables, direct-URL Python dependencies,
existing unowned MCP entries, and unknown action kinds are never executed or
overwritten. Unsupported work stays manual. Each executable permission and
target requires explicit user approval.

Assisted mutations must keep a parent transaction, per-step state, backup or
quarantine paths, hashes, failure reporting, and a recovery action. Automatic
recovery must refuse to overwrite any output that changed after apply and must
surface an exact manual recovery path instead. MCP configuration must be
fingerprinted again after the write-ahead checkpoint and at the final atomic
replacement boundary; any mismatch aborts without replacing the current file.
