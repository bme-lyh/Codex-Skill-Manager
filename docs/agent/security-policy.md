# Security policy

CSM is deny-by-default at the mutation boundary:

- only `https://github.com` and explicit local directories are accepted;
- archives reject path traversal and symbolic links;
- file count and file-size limits are enforced;
- `.system` is excluded and protected;
- static scanning runs before an installation plan is applyable;
- active critical findings block; findings are clustered by rule and file class;
- a cluster override must be manually reviewed, reasoned, persisted and journaled;
- deterministic baselines require an additional explicit manual confirmation;
- Codex review is advisory and can never issue or simulate a deterministic override;
- the GUI may batch-record applicable Codex suggestions only after an explicit
  human confirmation; deterministic members require a second confirmation and
  are still recorded as human overrides;
- active high findings require explicit acceptance;
- source commits and installed file hashes are recorded;
- GitHub secrets are not written to config, logs or reports.

Local scanning is the default. Codex CLI review is opt-in, runs against a bounded
review bundle in a read-only ephemeral session, strips common secret environment
variables and produces schema-validated output. Repository text is untrusted data.
Only a human decision can override a deterministic baseline.
