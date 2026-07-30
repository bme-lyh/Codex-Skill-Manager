# Extension guide

Keep extensions behind the existing package boundaries:

- add source types behind a provider that stages immutable content;
- add scanner rules with stable IDs and focused tests;
- expose use cases through `manager` before adding CLI or Wails bindings;
- add new state with schema migration rather than editing existing history;
- update both user and Agent documentation for public behavior.

Extensions must preserve explicit targeting, `.system` protection, offline
defaults, pre-change backup, reversible quarantine and JSON compatibility.

Adding an assisted-install action kind requires a local typed finalizer,
derived permission, explicit targets, plan-digest coverage, a no-shell executor,
transaction checkpoints, reverse recovery, hash-drift handling, and focused
tests. Never make a new action executable only by adding it to the Codex output
schema.
