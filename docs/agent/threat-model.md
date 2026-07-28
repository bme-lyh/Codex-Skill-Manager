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
- secret leakage through logs or configuration.

Mitigations include strict GitHub parsing, safe extraction, local rule scanning,
commit pinning, SHA-256 inventory, transactional backups, reversible quarantine,
credential-manager use and redacted operational reports. Static analysis cannot
prove semantic safety; user review remains required for high-risk code.
