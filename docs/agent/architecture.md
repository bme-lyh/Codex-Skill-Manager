# Architecture

CSM is a single Go codebase with two entry points:

- `cmd/csm`: deterministic CLI and JSON automation surface;
- repository root: Wails desktop application embedding the React build.

## Root model

Configuration schema 2 registers `codex-default` at
`%USERPROFILE%\.codex\skills` and `agents` at
`%USERPROFILE%\.agents\skills`. Codex is the default only for legacy callers;
current mutation APIs carry an explicit `rootId`. Skill, source, scan, group,
update, quarantine, restore, and transaction records keep that root identity.
Roots must be absolute, enabled, non-overlapping, and outside manager data
paths. Each root's top-level `.system` directory is read-only.

Core packages:

- `config`: validated absolute-path configuration;
- `inventory`: skill discovery, frontmatter parsing and SHA-256 inventory;
- `githubsource`: GitHub URL parsing, commit resolution and safe archive staging;
- `scanner`: local static security rules and severity policy;
- `manager`: use cases, locks, backups, quarantine, rollback, and typed assisted
  installation execution;
- `state`: SQLite transaction, scan and approval history;
- `reporting`: Markdown and JSON reader-facing reports;
- `scheduler`: Windows Task Scheduler integration for checks only;
- `auth`: GitHub token resolution and Windows Credential Manager storage.
- `codexreview`: opt-in CLI discovery, auth diagnostics, group-scoped risk review,
  full-repository installation analysis, monotonic JSONL activity progress,
  no-tool isolated execution, retry handling, and JSON-Schema-validated output.

`codexreview.Runner` is the shared process boundary for every Codex-backed
workflow. It owns executable discovery, environment filtering, timeouts,
bounded output, retries, JSONL diagnostics, and schema validation. Project
scans, install planning, security review, and related flows call this runner
instead of starting Codex independently.

The GUI calls the same manager facade as the CLI. The source lock is the
portable source of truth; SQLite is operational history. Filesystem changes,
lock updates and reports are tied together by transaction IDs.

Scanner findings remain immutable evidence. The manager decorates them into
stable clusters by effective group, Skill, rule, category and file class. A human cluster decision
expands to every member fingerprint. Multi-cluster decisions use one SQLite
transaction and one journal entry. Group operations persist one human approval
for Critical, High, Medium, and Low findings without a typed reason. Immutable
refs, paths, hashes, snapshot completeness, expiry, and recovery checks remain
non-bypassable. The backend resolves persisted findings and
severity instead of trusting client-supplied risk metadata. Model output is always
advisory. Codex review packages the complete selected group into the model input,
disables the shell tool, and runs from a manager-owned output directory. One
application group is one review task; all Skills in that group remain
together. Groups are serial by default, configurable concurrency is
bounded, and failed or incomplete group output is retried once serially. Static
findings are reduced to count-only rule overviews and remain leads rather than
conclusions.

Reusable group approvals are bound to the exact report plus root, group,
repository, commit, and policy version; a newer report or changed plan can
never reuse an older decision. Source-group parent transactions left running
by an application exit are reconciled on the next dashboard read into
`recovery-required` state, while completed child journals keep their own
recovery authority through the parent rollback entry.

SQLite `skill_security_states` stores the content hash, report ID and check time
for each successfully scanned Skill. Dashboard inventory hashes are compared with
that state so unchanged Skills can be skipped by default without hiding changed or
untracked content.

## Planned installation boundary

Every source first passes a persisted local `ProjectAssessment`, regardless of
whether Codex assistance is used. GitHub sources are bound to a full commit SHA;
local sources are copied into a bounded, manager-owned snapshot that rejects
links and special files. The preview digest binds the immutable source identity,
scan, candidates, and expiry. The assessment classifies the repository, inventories
documentation and installation markers, confirms Skill discovery and safe targets,
groups checks as required/triggered/optional, and returns a fail-closed
`ready`/`attention`/`blocked`/`incomplete` gate. Apply recomputes the assessment
from the bound source and refuses changed, expired, unknown, unsupported, or
case-variant `.system` targets.

Planned installation is a consent-gated manager workflow exposed through
the Wails desktop facade and the CLI's explicit `install --assist` contract. It
is layered on the ordinary installation preview. The standard resolver first pins GitHub
input to a full commit SHA and extracts it safely, or validates an explicit
local directory. It then discovers candidates and scans the exact Skill targets.
The reusable project-scan phase uses local results, bounded file summaries, and
a deterministic focused-file analysis. Every file is covered by an immutable
inventory and digest; credential-like files are metadata-only, large text files
are bounded, and each Codex input stays below the configured 800 KiB budget. Only
root-level VCS directories with real metadata markers are skipped; ordinary
same-name and manager-like directories remain in scope. Canonical containment,
handle identity, and post-read checks reject escape or replacement during
packaging. If the package exceeds one model request, deterministic chunks cover
every eligible text file exactly once and a final no-tools pass synthesizes the
chunk results. It returns an overview, security conclusion, and declarative
installation methods but cannot create permissions or execution steps. A
separate explicit user decision authorizes plan creation from the verified scan.
The assisted session disables Codex's shell tool and uses a manager-owned output
directory as its working directory. A context digest detects changes before
apply.

Model output is proposal data, never execution authority. A local finalizer
validates the source and plan digests, rejects model-supplied paths or
environment variables, derives permissions, and reduces actions to this
allowlist:

- `install-skills`: apply selected candidates through the normal install
  transaction;
- `managed-python-tool`: verify repository ownership through official PyPI
  metadata, resolve the complete Wheel closure in isolated staging, lock each
  name/version/filename/SHA-256, reject source builds and incompatible
  artifacts, and install the approved lock offline into an application-owned
  environment;
- `configure-codex-mcp`: point a new manager-owned MCP entry at the managed
  executable with a fixed `serve` argument and an explicit version-controlled
  project root;
- `manual`: display only.

Unknown and free-form actions become manual work. A proposal that
supplies a path or environment is rejected. Repository scripts, free-form shell,
arbitrary package managers, environment injection, and model-selected write
paths are never executed. The UI must show the repository summary,
requirements, typed plan, derived permissions, and monotonic progress before
and during execution. The desktop relays live progress and cancellation; CLI
JSON returns the final plan or result. Approved automatic steps may complete
before a `partial` terminal result lists every required manual step. Managed
Python/MCP execution also requires a GitHub source so official PyPI metadata can
verify ownership. Native-code Wheels require a separate high-risk permission.

Each execution has a parent transaction whose steps retain targets, child
transactions, backups, hashes, and recovery status. Completed reversible steps
recover in reverse order after failure. Hash drift prevents automatic recovery
from overwriting a user change and produces an explicit manual recovery path.
