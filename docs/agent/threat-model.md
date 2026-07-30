# Threat model

Protected assets are the user's existing skills, GitHub credentials, source
provenance, local files and audit history.

Primary threats:

- malicious or compromised repositories;
- archive traversal, symlinks and oversized payloads;
- prompt instructions that request destructive or credential-exfiltrating work;
- shell, network, persistence and dynamic-execution primitives;
- name collisions that overwrite a skill from another source;
- partial multi-skill updates and tampered local files;
- repository instructions or model output attempting to turn assisted
  installation into arbitrary command execution;
- dependency confusion, source builds, archive bombs, or a Python package whose
  published metadata does not match the analyzed repository;
- stale assisted plans, Codex configuration races, a wrong MCP working
  directory, and partial cross-system mutation;
- secret leakage through logs or configuration.

Mitigations include strict GitHub parsing, safe extraction, local rule scanning,
commit pinning, SHA-256 inventory, transactional backups, reversible quarantine,
credential-manager use and redacted operational reports. Static analysis cannot
prove semantic safety; user review remains required for high-risk code.

Assisted installation adds a separate model-output trust boundary. The manager
packages the complete commit-pinned GitHub staging directory or explicit local
source into a Codex session whose shell tool is disabled and whose working
directory contains only manager-owned output files. Codex emits
schema-constrained proposal data. The local
finalizer rejects paths, environment, and free-form commands; only allowlisted
typed steps with derived, explicitly approved permissions may execute. Python
tools require official PyPI repository matching, exact Wheel-only resolution,
bounded archive inspection, an app-launched pip process forced through a
PyPI-only loopback proxy,
offline installation into an application-owned environment, and a sanitized
process environment. MCP configuration requires a real version-controlled
project root, an app-owned executable, a fixed argument, a non-conflicting
server name, and a matching pre-apply configuration hash. The configuration
snapshot is rechecked after the transaction checkpoint and immediately before
the atomic replacement so a concurrent edit stops the mutation.

The proxy is not an operating-system sandbox. The selected local Python and pip
remain trusted computing-base components because a compromised executable could
open a raw connection outside its configured proxy.

Parent/child transaction journals, backups, output hashes, reverse recovery and
quarantine limit partial-write damage. Hash drift blocks automatic overwrite
and turns recovery into an explicit manual action. These controls reduce risk;
they do not make an arbitrary complex repository safe or fully automatable.
