# Extension guide

Keep extensions behind the existing package boundaries:

- add source types behind a provider that stages immutable content;
- add scanner rules with stable IDs and focused tests;
- expose use cases through `manager` before adding CLI or Wails bindings;
- add new state with schema migration rather than editing existing history;
- update both user and Agent documentation for public behavior.

Extensions must preserve explicit targeting, `.system` protection, offline
defaults, pre-change backup, reversible quarantine and JSON compatibility.
