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
- `codexreview`: opt-in CLI discovery, auth diagnostics, group-scoped review
  tasks, per-Skill results, monotonic JSONL activity progress, read-only execution,
  retry handling and JSON-Schema-validated summaries.

The GUI calls the same manager facade as the CLI. The source lock is the
portable source of truth; SQLite is operational history. Filesystem changes,
lock updates and reports are tied together by transaction IDs.

Scanner findings remain immutable evidence. The manager decorates them into
stable clusters by effective group, Skill, rule, category and file class. A human cluster decision
expands to every member fingerprint. Multi-cluster decisions use one SQLite
transaction and one journal entry. Reasons are optional, deterministic rules use
the same human action as every other severity, and model output is always
advisory. Codex review runs with the complete target as its read-only working
directory. One application group is one review task; all selected Skills in that
group remain together. Groups are serial by default, configurable concurrency is
bounded, and failed or incomplete group output is retried once serially. Static
findings are reduced to count-only rule overviews and remain leads rather than
conclusions.

SQLite `skill_security_states` stores the content hash, report ID and check time
for each successfully scanned Skill. Dashboard inventory hashes are compared with
that state so unchanged Skills can be skipped by default without hiding changed or
untracked content.
