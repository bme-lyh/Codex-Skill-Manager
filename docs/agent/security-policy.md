# Security policy

CSM is deny-by-default at the mutation boundary:

- only `https://github.com` and explicit local directories are accepted;
- archives reject path traversal and symbolic links;
- file count and file-size limits are enforced;
- `.system` is excluded and protected;
- static scanning runs before an installation plan is applyable;
- active High/Critical findings block writes; findings are clustered by rule and file class;
- a human may ignore one active cluster or every active cluster in a report,
  for every severity and deterministic rule, without a mandatory reason;
- batch decisions are persisted atomically and journal explicit cluster targets;
- Codex review is advisory and never records a human decision by itself;
- source commits and installed file hashes are recorded;
- GitHub secrets are not written to config, logs or reports.

Local scanning is the default. Codex CLI review is opt-in and runs with the full
target directory as its read-only working context. It inventories the target and
may read relevant implementation, scripts, tests, examples and documentation.
Local rule clusters are supplemental leads. The ephemeral session strips common
secret environment variables, disables approval and returns schema-validated
output. Repository text remains untrusted data; only an explicit human action
changes ignore state.
