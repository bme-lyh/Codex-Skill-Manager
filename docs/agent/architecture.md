# Architecture

CSM is a single Go codebase with two entry points:

- `cmd/csm`: deterministic CLI and JSON automation surface;
- repository root: Wails desktop application embedding the React build.

Core packages:

- `config`: validated absolute-path configuration;
- `inventory`: skill discovery, frontmatter parsing and SHA-256 inventory;
- `githubsource`: GitHub URL parsing, commit resolution and safe archive staging;
- `scanner`: local static security rules and severity policy;
- `manager`: use cases, locks, backups, quarantine and rollback orchestration;
- `state`: SQLite transaction, scan and approval history;
- `reporting`: Markdown and JSON reader-facing reports;
- `scheduler`: Windows Task Scheduler integration for checks only;
- `auth`: GitHub token resolution and Windows Credential Manager storage.
- `codexreview`: opt-in CLI discovery, auth diagnostics, read-only review
  execution and JSON-Schema-validated summaries.

The GUI calls the same manager facade as the CLI. The source lock is the
portable source of truth; SQLite is operational history. Filesystem changes,
lock updates and reports are tied together by transaction IDs.

Scanner findings remain immutable evidence. The manager decorates them into
stable clusters by rule, category and file class. A human cluster decision
expands to every member fingerprint. Deterministic overrides require a distinct
confirmation and approval record; model output is always advisory.
